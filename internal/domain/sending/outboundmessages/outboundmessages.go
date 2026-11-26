// Package outboundmessages owns the Outbound messages and is the one place a message leaves sphericon on a Workspace's behalf, on any
// Channel (ADR 0015, ADR 0079). Every surface — Broadcast, Automation step, Transactional — hands
// it a Request and gets back a typed Outcome; none re-implements any part of what lies between
// "this message is wanted" and "the provider accepted it". The pipeline is one, in Send:
//
//   - replay: an already-recorded send returns its recorded Outcome (idempotency);
//   - holds: Workspace freeze, then the Send route of the Request's Channel and the Channel's own
//     check (email: an unverified Sending domain) — reversible, per-source, never consumed as a
//     per-recipient failure;
//   - the targets: where the message goes on its Channel, read only;
//   - Send-eligibility of each target, fail-closed: a target it forbids is dropped, and when none is
//     left the send is Skipped (final for that destination), recorded as a decided message under
//     the Request's idempotency key with no claim taken first;
//   - claim: the Outbound message row is taken before the provider is called, under
//     the Request's idempotency key, with a lease against racing retries; a new claim of a
//     marketing message is checked against the Frequency cap of each target and written with its
//     count, under the lock of each Destination a cap holds (frequencycap, ADR 0178): a target over
//     its cap is dropped, and when none is left the send is Skipped, recorded as a decided message;
//   - render, the Provider limit and the Send cap, the provider call;
//   - the record of the result and the Channel's sent Event, in one transaction.
//
// The Transactional send itself is the module's too (SendTransactional, transactional.go, ADR
// 0112): it reads the Template the request references and resolves its Contact, then sends
// through Send.
//
// Only a Channel's renderer and provider adapter differ (channel.go). Email's (email.go) holds
// the Sending-domain gate, the unsubscribe footer and RFC 8058 headers, the open pixel and DKIM
// signing (inside the EmailSender); a Test send and the System sender are email's (testsend.go).
package outboundmessages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"

	"github.com/samber/lo"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/ratelimit"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// Outcome is what an Outbound send did. Sent, Skipped and Failed are final for the
// destination; Held and Deferral are reversible per-source Outcomes — nothing was
// consumed, the caller should try the same Request again later (a Held after its own
// delay, a Deferral after Result.Wait).
type Outcome string

const (
	// Sent: the provider accepted the message (not "delivered" — see GLOSSARY).
	Sent Outcome = "sent"
	// Skipped: the send is final for this destination (Send-eligibility forbids it, it has none).
	// Result.SkippedReason says why.
	Skipped Outcome = "skipped"
	// Failed: this one message can never be sent (e.g. its template failed to render).
	// Result.Failure says what it failed with.
	Failed Outcome = "failed"
	// Held: the source cannot send right now. Result.Hold says why.
	Held Outcome = "held"
	// Deferral: a send limit is spent — the Integration's Provider limit (ADR 0023), the
	// Workspace's Send cap (ADR 0067) or the provider's own. Nothing is wrong with the source,
	// it is busy: no hold reason, no claim, no attempt consumed. Result.Wait says when
	// capacity returns and Result.Cause which limit it waits for.
	Deferral Outcome = "deferral"
	// Refused: a Test send the System sender will not make (ADR 0041). Result.Refusal is a Refuse*
	// constant; unlike a Held it does not clear by waiting.
	Refused Outcome = "refused"
	// Withdrawn: the source stopped taking new messages before this one was claimed or decided
	// (Request.Stopped, a cancelled Broadcast, ADR 0150). Nothing was sent or recorded, and it is
	// final: the same Request is withdrawn again.
	Withdrawn Outcome = "withdrawn"
)

// Backoffs for a provider reply that says it is busy (ADR 0023). The core does not
// auto-tune the Provider limit; it only waits. A daily quota refills gradually, so
// its wait is long.
const (
	DeferralBackoff = 30 * time.Second
	QuotaBackoff    = time.Hour
)

// deferralBackoff is the wait a provider "too fast" or "quota exceeded" error asks
// for, zero when err is neither.
func deferralBackoff(err error) time.Duration {
	if d, ok := errors.AsType[deferError](err); ok {
		return d.wait
	}
	switch {
	case errors.Is(err, messaging.ErrQuotaExceeded):
		return QuotaBackoff
	case errors.Is(err, messaging.ErrBusy):
		return DeferralBackoff
	default:
		return 0
	}
}

// ReasonMessageRefused starts the failure of a message the provider refused for its own
// Destination, or with an answer that concerns no one else (an invalid number, a code that tells
// nothing): "message_refused: <the provider's words> (error <code>)". The message fails alone: the
// source's other messages, and an Automation's next steps, go on (Result.MessageOnly).
const ReasonMessageRefused = "message_refused"

// Reasons a Test send from the System sender is Refused (ADR 0041).
const (
	// RefuseOtherRecipient: the Workspace has no verified Sending domain, so a Test send
	// may only go to the address of the User who asks.
	RefuseOtherRecipient = "other_recipient"
	// RefuseNoSystemSender: the Instance has no outbound mail, so there is no System sender
	// (or one that names no From to send from).
	RefuseNoSystemSender = "no_system_sender"
)

// HoldDetail words a Hold reason for a human (an API problem detail). The single place the wording
// lives, so every surface that reports a Held outcome agrees.
func HoldDetail(reason holdreason.Reason) string {
	if msg := holds.Message(reason); msg != "" {
		return msg
	}
	switch reason {
	case holdreason.NoIntegration:
		return "no integration of the channel can send: the one named is disabled, archived or of another channel, or the channel has no default"
	case holdreason.UnverifiedDomain:
		return "sender domain is not a verified sending domain"
	case holdreason.ProviderStopped:
		return "the email provider has stopped sending from this account"
	case holdreason.ProviderRefused:
		return "the provider refused the integration's credentials or account: save the integration again with working ones"
	default:
		return "sending is currently on hold for this workspace: " + string(reason)
	}
}

// RefuseDetail words the reason a Test send was Refused for a human, as HoldDetail does a Hold's.
func RefuseDetail(reason string) string {
	switch reason {
	case RefuseOtherRecipient:
		return "until a sending domain is verified, a test send can only go to your own address"
	case RefuseNoSystemSender:
		return "outbound mail is not configured on this instance, so a test send needs a verified sending domain"
	default:
		return "a test send was refused: " + reason
	}
}

// ErrInProgress means another attempt holds a live claim on this idempotency key.
// It is retryable; a stale claim (older than the lease) is taken over instead.
var ErrInProgress = errors.New("outbound: a send with this key is already in progress")

// ErrNoTracker means a marketing send (a Request with a Source) was attempted
// without the Tracker that builds its unsubscribe link. A marketing message must
// never leave without one (ADR 0012), so this is a configuration error, never a
// silent downgrade.
var ErrNoTracker = errors.New("outbound: marketing send requires an unsubscribe tracker")

// Result is the outcome of a Send.
type Result struct {
	Outcome Outcome
	// Wait is, for a Deferral, how long until capacity returns.
	Wait time.Duration
	// Cause is, for a Deferral of a Send, the limit it waits for; empty for every other Outcome
	// and for a Test send (its cap is Limit).
	Cause deferralcause.Cause
	// SkippedReason is why a Skipped send was skipped.
	SkippedReason skipreason.Reason
	// Failure is what a Failed send failed with.
	Failure string
	// Refusal is why a Test send was Refused, a Refuse* constant.
	Refusal string
	// Hold is why a Held send is held.
	Hold holdreason.Reason
	// MessageID is the Outbound message row; zero when Held (nothing was recorded).
	MessageID int64
	// Replayed is true when the Outcome was read back from an earlier attempt.
	Replayed bool
	// From is, for a Test send that was Sent, the From address it went out from.
	From string
	// Captured is true for a Test send that went through a sender that delivers nowhere
	// outside: the message was kept, not sent to the address named (ADR 0058).
	Captured bool
}

// MessageOnly reports whether a Failed is this message's alone (ReasonMessageRefused), so the source
// goes on past it. It reads the recorded failure, so a replayed Result answers as the first did.
func (r Result) MessageOnly() bool {
	return r.Outcome == Failed && strings.HasPrefix(r.Failure, ReasonMessageRefused+":")
}

// Senders builds the EmailSender email's provider adapter sends through, for the Integration a
// Send route resolved; a nil one is ErrNoProvider. *messaging.Resolver satisfies it.
type Senders interface {
	EmailSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error)
}

// Users finds the User who asks for a Test send, whose own address is the only one a Test send
// from the System sender may go to (ADR 0041). *users.Module satisfies it.
type Users interface {
	User(ctx context.Context, id int64) (*ent.User, error)
}

// TestSendBudget meters Test sends from the System sender, per User and per Workspace
// (ADR 0041, ADR 0025). TakeTestSend reserves one slot of each and returns the zero Refusal
// when it was granted; otherwise the Refusal says how long until a slot frees and which cap
// was hit.
type TestSendBudget interface {
	TakeTestSend(ctx context.Context, userID, workspaceID int64) (ratelimit.Refusal, error)
}

// DefaultLease is how long a pending claim blocks other attempts before a retry may
// take it over. It must outlast every attempt that can hold a claim (ADR 0015, ADR 0144): a send
// job, under its timeout, and a Transactional request. A shorter lease lets a retry take over a
// send that is still live, and the provider gets the message twice.
const DefaultLease = 5 * time.Minute

// Module is the Outbound send module.
type Module struct {
	bus *events.Bus
	// channels are the Channels a message can be sent on, each with its renderer and adapter.
	channels map[channels.Channel]channel
	senders  Senders
	// bots builds the Telegram bots; nil while the Instance has none.
	bots TelegramBots
	// sms builds the SMS senders and hooks names where an SMS Integration's delivery reports are
	// posted; nil while the Instance has none.
	sms     SMSSenders
	hooks   HookURLs
	tracker *messagetoken.Tracker
	// trackingHosts names the Workspace's serving Tracking domain; nil while the Instance offers
	// none.
	trackingHosts TrackingHosts
	// sendCaps is whether the Instance has Send caps at all (ADR 0067); without them a
	// Workspace's stored Send cap is ignored.
	sendCaps providerlimit.SendCaps
	lease    time.Duration
	holds    *holds.Holds
	// buckets spends the Provider limit and the Send cap, limits reads them: both at now.
	buckets *providerlimit.Buckets
	limits  *providerlimit.Module
	// standing records the provider's refusal of an Integration's credentials; chats unbinds a
	// chat Telegram no longer reaches. Both publish or write in the send's transaction.
	standing *integrations.Standing
	chats    *bindings.Chats
	// system is the Instance's System sender; nil on an Instance with no outbound mail.
	system messaging.EmailSender
	// testBudget caps Test sends from the System sender; nil means uncapped.
	testBudget TestSendBudget
	// users finds the User a Test send is asked by; nil on a module that sends no Test send for one.
	users Users
	// startLinks issues the Start tokens of {{ telegram_start_url }}; nil renders the tag empty.
	startLinks StartLinks
	// now is the clock the Provider limit and the claim lease are read at.
	now func() time.Time
	// eligibility decides whether a target may be sent to (stateless, built by New); contacts
	// resolves the Contact of a Transactional send's address.
	eligibility *eligibility.Module
	contacts    *contacts.Module
	// consents answers a Destination's effective Engagement tracking consent (ADR 0120).
	consents *engagementtracking.Module
	// caps holds the Destination locks of a claim and its count (ADR 0178).
	caps *frequencycap.Module
}

// TrackingHosts names the host a Workspace's links are built on: its current Tracking domain
// while it serves, "" otherwise (ADR 0071).
type TrackingHosts interface {
	ServingHost(ctx context.Context, s *ent.Scoped) (string, error)
}

// Modules are the modules every send reads or writes through, each the one the wiring built for
// the rest of the Instance; none may be nil. A test builds them over its own bus and clock when it
// has its own (the chats publish in the send's transaction, so their bus is the module's).
type Modules struct {
	// Contacts resolves the Contact of a Transactional send's address.
	Contacts *contacts.Module
	// Chats unbinds a chat Telegram no longer reaches.
	Chats *bindings.Chats
	// Limits reads the Provider limit's 24-hour usage and the Send cap's drain.
	Limits *providerlimit.Module
	// Standing records the provider's refusal of an Integration's credentials.
	Standing *integrations.Standing
	// Consents answers whether a Destination consents to being tracked (ADR 0120).
	Consents *engagementtracking.Module
	// Caps takes the Destination locks a claim is written under and counts a marketing message at
	// each Destination it is claimed for (ADR 0178).
	Caps *frequencycap.Module
}

// Deps are the collaborators of the module's optional parts, each passed by the wiring; a nil one
// turns its part off, and a test leaves nil what it does not reach.
type Deps struct {
	// Bots builds the bots Telegram messages are sent through. Nil, the Telegram Channel has no
	// Integration that can send.
	Bots TelegramBots
	// SMS builds the senders SMS messages go through and Hooks names where their delivery reports
	// are posted. Without both, the SMS Channel has no Integration that can send.
	SMS   SMSSenders
	Hooks HookURLs
	// TrackingHosts turns on Tracking domains (SPHERICON_TRACKING_CNAME_TARGET is set): every
	// tracked message of a Workspace whose Tracking domain serves gets its links on that host. Nil,
	// every link uses the Instance's host (ADR 0038, ADR 0071).
	TrackingHosts TrackingHosts
	// System is the Instance's System sender, which a Test send falls back to while the Workspace
	// has no verified Sending domain (ADR 0041); nil on an Instance with no outbound mail.
	System messaging.EmailSender
	// TestBudget caps the Test sends made from the System sender; nil leaves them uncapped.
	TestBudget TestSendBudget
	// Users finds the User a Test send is asked by, the scope's actor (ADR 0041); nil on a module
	// that sends no Test send for one.
	Users Users
	// StartLinks issues the Start tokens of {{ telegram_start_url }}; nil renders the tag empty.
	StartLinks StartLinks
}

// Config is what the module needs besides its collaborators.
type Config struct {
	// SendCaps says whether the Instance has Send caps (ADR 0067): with them every send of a
	// Workspace that has one also spends from its 24-hour bucket. Without (the zero value), a
	// stored Send cap has no effect.
	SendCaps providerlimit.SendCaps
	// Lease is how long a pending claim blocks other attempts; zero is DefaultLease (tests use a
	// tiny one to exercise takeover).
	Lease time.Duration
}

// Now reads the module's clock (New's now): the moment a Deferral's Wait is measured from, so a
// caller that records when the deferred send is due uses the same clock.
func (m *Module) Now() time.Time { return m.now() }

// AsSystem returns s acting as the `system` actor (ADR 0022), for a job or automation
// step that changes an audited entity in the normal course of work.
func (m *Module) AsSystem(s *ent.Scoped) *ent.Scoped {
	return m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
}

// New builds the module. tracker may be nil only if no marketing Request is sent; mods are the
// modules every send goes through, deps the collaborators of its optional parts and cfg its
// settings. now is the Instance clock (app.Clock)
// the module reads for the Provider limit's token buckets, the claim lease and the moment a
// message is recorded sent; tests move it instead of waiting.
func New(bus *events.Bus, hs *holds.Holds, senders Senders, tracker *messagetoken.Tracker, mods Modules, deps Deps, cfg Config, now func() time.Time) *Module {
	m := &Module{
		bus: bus, holds: hs, senders: senders, tracker: tracker,
		bots: deps.Bots, sms: deps.SMS, hooks: deps.Hooks, trackingHosts: deps.TrackingHosts,
		system: deps.System, testBudget: deps.TestBudget, users: deps.Users, startLinks: deps.StartLinks,
		sendCaps: cfg.SendCaps, lease: cmp.Or(cfg.Lease, DefaultLease), now: now,
		buckets: providerlimit.NewBuckets(bus.WithinAuditTx, now), limits: mods.Limits,
		standing: mods.Standing, chats: mods.Chats,
		eligibility: eligibility.New(), contacts: mods.Contacts, consents: mods.Consents, caps: mods.Caps,
	}
	m.channels = map[channels.Channel]channel{channels.Email: emailChannel{m: m}, channels.Telegram: telegramChannel{m: m}, channels.SMS: smsChannel{m: m}}
	return m
}

// channel is the part of the send of ch, an error for a Channel no message is sent on.
func (m *Module) channel(ch channels.Channel) (channel, error) {
	c, ok := m.channels[ch]
	if !ok {
		return nil, fmt.Errorf("outbound: no message is sent on channel %q", ch)
	}
	return c, nil
}

// Request describes one message to send on one Channel. Content is the already-bound Message
// content, whose Channel is the send's (a marketing surface passes its own copy, Transactional
// the live Template's current content — see ADR 0003, 0005); the module never looks it up.
type Request struct {
	// Key is unique per logical send: a retry of the same send repeats it. Required.
	Key string

	// Destination is where the message goes on its Channel (on email the address). Contact, when
	// the destination resolves to one, supplies merge tags and attribution; ContactID alone
	// attributes only. A send to a Contact may leave it empty when the Contact has no Destination
	// on the Channel: the send is then Skipped with skipreason.NoDestination.
	Destination string
	Contact     *ent.Contact
	ContactID   int64

	// Content is the Message content: its Channel is the Channel the message goes out on.
	// Required.
	Content Content
	// Variables are extra merge tags; they win over Contact fields.
	Variables map[string]any

	// IntegrationID is the Integration of the Channel the message goes through; nil is that
	// Channel's default. Its Send route decides: one that cannot send, or one of another Channel,
	// holds the message (no_integration), never falling back to the default.
	IntegrationID *int64

	// Track turns on open/click tracking for this message: the tracking token names the
	// Outbound message (ADR 0053).
	Track bool

	// Ref is the surface record the message is sent for, and so its kind and its mode of
	// Send-eligibility. Required.
	Ref Ref

	// Stopped, when set, says when the source stopped taking new messages: the surface that owns
	// it reads its own state (a cancelled Broadcast, ADR 0150). It is asked once a new claim, or a
	// Skipped decided before the claim, is written, so a stop that raced the write is seen: one
	// committed before the question withdraws the row (Withdrawn, nothing recorded), and one
	// committed after it finds the row, a message that goes out or stays Skipped. A claim another
	// attempt left behind is asked too, once its lease has run out: one written after the stop is a
	// withdrawal that never finished, and is withdrawn instead of taken over; one written before it
	// goes on.
	Stopped SourceStop

	// Waited is how the send has waited at its place before this attempt, which its Outbound
	// message keeps once decided (ADR 0178); the zero value for a send that has not waited.
	Waited Waited

	// digest is what a Transactional request with the caller's key asked for (requestDigest); ""
	// for every other send. Only Transactional sets it.
	digest string
}

// Kind is the surface that asked for the send: its Ref's.
func (r Request) Kind() outboundmessage.Kind { return r.Ref.Kind() }

// Mode is what Send-eligibility checks the message for: its Ref's.
func (r Request) Mode() eligibility.Mode { return r.Ref.mode() }

// source is the unsubscribe Sending source of a marketing message ("broadcasts",
// "automation:<id>"), "" for a message that carries none.
func (r Request) source() string { return r.Mode().Source() }

// broadcastID is the Broadcast the message is sent for, 0 for none.
func (r Request) broadcastID() int64 {
	b, _ := r.Ref.owners()
	return b
}

// automationID is the Automation the message is sent for, 0 for none.
func (r Request) automationID() int64 {
	_, a := r.Ref.owners()
	return a
}

// Channel is the Channel the message goes out on: its content's.
func (r Request) Channel() channels.Channel { return r.Content.Channel() }

// contactID is the Contact the message is attributed to, 0 for none.
func (r Request) contactID() int64 {
	if r.Contact != nil {
		return r.Contact.ID
	}
	return r.ContactID
}

func (r Request) validate() error {
	switch {
	case r.Key == "":
		return errors.New("outbound: idempotency key required")
	case r.Content == nil:
		return errors.New("outbound: content required")
	case r.Ref == nil:
		return errors.New("outbound: ref required")
	case emailcase.Of(r.Destination) == "" && r.Contact == nil:
		return errors.New("outbound: destination required")
	}
	return nil
}

// Send performs one Outbound send. A returned error is a retryable infrastructure
// failure (provider unreachable, database error, ErrInProgress); every decided
// outcome — including Skipped, Failed and Held — is a Result with a nil error.
func (m *Module) Send(ctx context.Context, s *ent.Scoped, req Request) (Result, error) {
	if err := req.validate(); err != nil {
		return Result{}, err
	}
	if req.source() != "" && m.tracker == nil {
		return Result{}, ErrNoTracker
	}
	onChannel := req.Channel()
	ch, err := m.channel(onChannel)
	if err != nil {
		return Result{}, err
	}
	dest := emailcase.Of(req.Destination)

	// A recorded send replays its Outcome — even while the Workspace is frozen: the
	// message already left (or was already decided), nothing new is being sent.
	existing, err := m.find(ctx, s, req.Key)
	if err != nil {
		return Result{}, err
	}
	if err := sameRequest(existing, req); err != nil {
		return Result{}, err
	}
	if existing != nil && existing.Status != outboundmessage.StatusPending {
		return replayResult(existing), nil
	}

	// The Contact must still exist: a send queued before its Erasure is dropped here,
	// the one chokepoint every worker passes (ADR 0021), before anything is claimed or
	// recorded that would keep the erased person's address.
	if gone, err := m.contactErased(ctx, s, req); err != nil {
		return Result{}, err
	} else if gone {
		// Nothing is recorded for it: the message row would carry the erased person's address.
		return Result{Outcome: Skipped, SkippedReason: skipreason.ContactErased}, nil
	}

	ws, err := m.workspace(ctx, s)
	if err != nil {
		return Result{}, err
	}
	g, err := m.gate(ctx, s, req.Content, req.IntegrationID)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{Outcome: Held, Hold: g.hold}, nil
	}

	// Where the message goes is the Channel's to resolve, read only and before the claim: a Channel
	// of Bindings reads the Contact's Bindings of the routed Integration. A Contact with no
	// Destination on the Channel has nothing to send to, and a target Send-eligibility forbids is
	// dropped: when none is left, the send is Skipped, recorded as a decided message under the key
	// with no claim taken first, after the holds (ADR 0079, 0081, 0178).
	eligible, skip, err := m.recipients(ctx, s, req, ch, g, dest)
	if err != nil {
		return Result{}, err
	}
	if len(eligible) == 0 {
		return m.skipUnclaimed(ctx, s, req, dest, g, existing, skip)
	}

	// The Sending window and the Frequency cap sit here, between Send-eligibility and the claim
	// (ADR 0178): under the lock of each Destination a cap holds, a target over its cap is dropped and
	// a new claim is written to the rest with its count; with none left, the send is Skipped
	// (claimNew).
	c, replay, err := m.claim(ctx, s, req, dest, g, existing, eligible)
	if err != nil {
		return Result{}, err
	}
	if replay != nil {
		return *replay, nil
	}
	msg, eligible := c.msg, c.targets
	if len(eligible) == 0 {
		// A claim taken over whose every Destination with a place is gone or ineligible since.
		return m.skip(ctx, s, msg, skipreason.NoDestination)
	}
	dest = eligible[0].dest

	// Whether the message goes out tracked is decided now, after the claim, so a refusal recorded
	// meanwhile finds the row (ADR 0120); the decision is on the row before the provider is called.
	tracked, err := m.decideTracking(ctx, s, req, ch, msg, eligible)
	if err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	}

	// What the render decided is the message's fact, recorded before the provider is called: a
	// message whose render failed decided nothing.
	rendered, err := m.renderDecided(ctx, s, req, ch, msg, dest, ws, tracked)
	if err != nil {
		if !isContentError(err) {
			// The storage behind the content failed, not the content, or the claim was taken over:
			// retry rather than record a message that could have been sent as Failed for good.
			m.release(ctx, s, msg)
			return Result{}, err
		}
		return m.fail(ctx, s, msg, err.Error())
	}

	// Provider limit and Send cap: the last gate before the provider, so a message that was
	// Skipped, Held or failed to render never spends capacity.
	if short, err := m.reserve(ctx, s, req, g.route, ws); err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	} else if short.Wait > 0 {
		// Busy, not blocked: drop the claim so the retry starts clean and no attempt
		// is consumed.
		m.dropClaim(ctx, s, msg)
		return Result{Outcome: Deferral, Cause: short.Cause, Wait: short.Wait}, nil
	}

	origin := messaging.Origin{OutboundMessageID: msg.ID, Surface: messaging.Surface(req.Kind()), Source: req.source()}
	first, stopped, skip, err := m.deliver(ctx, s, g, req, rendered, eligible, origin, msg)
	if err != nil {
		if stopped != nil {
			m.dropClaim(ctx, s, msg)
			return *stopped, nil
		}
		if ce, ok := errors.AsType[contentError](err); ok {
			// The provider refused the message itself, as it will on every attempt.
			return m.fail(ctx, s, msg, ce.Error())
		}
		m.release(ctx, s, msg)
		return Result{}, err
	}
	if first == nil {
		// No target took the message. What the provider said of them is recorded (an opt-out, a
		// Binding gone, a suppressed number), and the message is Skipped with what Send-eligibility
		// would now say.
		if skip == "" {
			skip = skipreason.NoDestination
		}
		return m.skip(ctx, s, msg, skip)
	}
	return m.recordSent(ctx, s, req, msg, g, first.to.dest, first.receipt, renderedSubject(rendered))
}

// renderedSubject is the subject line of the rendered message, "" for a Channel whose messages
// have none: what the recipient received, kept on the Outbound message (#899).
func renderedSubject(rendered any) string {
	if email, ok := rendered.(messaging.EmailMessage); ok {
		return email.Subject
	}
	return ""
}

// recipients is the targets of req that Send-eligibility allows, read only: those its Channel
// resolves through the route g (dest is the Request's own Destination), each checked on its own.
// When none is left, skip says why: no_destination when the Channel resolves none.
func (m *Module) recipients(ctx context.Context, s *ent.Scoped, req Request, ch channel, g gateResult, dest string) (eligible []target, skip skipreason.Reason, err error) {
	targets, err := ch.targets(ctx, s, req, g.route, dest)
	if err != nil {
		return nil, "", err
	}
	if len(targets) == 0 {
		return nil, skipreason.NoDestination, nil
	}
	return m.eligibleTargets(ctx, s, req, targets)
}

// eligibleTargets is the targets that pass Send-eligibility, each checked on its own at send time,
// fail-closed: an error is a check that could not be made. When none passes, skip is the first
// one's reason.
func (m *Module) eligibleTargets(ctx context.Context, s *ent.Scoped, req Request, targets []target) (eligible []target, skip skipreason.Reason, err error) {
	onChannel := req.Channel()
	for _, t := range targets {
		dec, err := m.eligibility.Check(ctx, s, eligibility.CheckInput{Channel: onChannel, Destination: t.dest, Mode: req.Mode()})
		if err != nil {
			return nil, "", err
		}
		if dec.Eligible {
			eligible = append(eligible, t)
		} else if skip == "" {
			skip = dec.Reason
		}
	}
	return eligible, skip, nil
}

// deliver hands the rendered message to each target in turn and returns the first the provider
// accepted. A target that fails after one has accepted is not retried, which would send the first
// again. Before any has: stopped is the Held or Deferral Result the failure means (the caller drops
// the claim), else the error is retryable, except a contentError, which is the message's own. A
// target the provider says reaches no one is dropped (its Channel's unreachable, skip is the reason
// it now has) and one whose person stopped at the provider (a blocked bot, a number that texted STOP
// to the sender) is their opt-out (recordOptedOut, skip is unsubscribed_everything); one that refuses
// the message for good, for that target alone (chatRefusedError), is a failed target only. The
// message goes on to the others: first is nil when none took it and none failed, and a contentError
// with the first refusal's description when none took it and one refused.
func (m *Module) deliver(ctx context.Context, s *ent.Scoped, g gateResult, req Request, rendered any, targets []target, origin messaging.Origin, msg *ent.OutboundMessage) (first *accepted, stopped *Result, skip skipreason.Reason, err error) {
	messageID := msg.ID
	var refused *chatRefusedError
	for i, t := range targets {
		if i > 0 {
			// The first call is what reserve took a token for.
			m.spendCall(ctx, s, g.route, messageID)
		}
		receipt, err := g.route.Sender.send(ctx, g.as, rendered, t, origin)
		if err == nil {
			if first == nil {
				first = &accepted{to: t, receipt: receipt}
			}
			continue
		}
		if _, stopped := errors.AsType[optedOutError](err); stopped {
			if err := m.recordOptedOut(ctx, s, req, g, msg, t); err != nil {
				return first, nil, skip, err
			}
			// The person's opt-out outweighs a dropped target's reason.
			skip = skipreason.UnsubscribedEverything
			continue
		}
		if _, gone := errors.AsType[unreachableError](err); gone {
			reason, err := m.dropTarget(ctx, s, req, g, t, messageID)
			if err != nil {
				return first, nil, skip, err
			}
			if skip == "" {
				skip = reason
			}
			continue
		}
		if cr, ok := errors.AsType[chatRefusedError](err); ok {
			if refused == nil {
				refused = &cr
			}
			continue
		}
		if first != nil {
			slog.WarnContext(ctx, "outbound: a target did not take the message", "message_id", messageID, "err", err)
			continue
		}
		if stop, serr := m.stopOf(ctx, s, g, req, err); stop != nil || serr != nil {
			return nil, stop, skip, serr
		}
		if isContentError(err) {
			return nil, nil, skip, err
		}
		return nil, nil, skip, fmt.Errorf("outbound: send to %s: %w", t.dest, err)
	}
	if first == nil && refused != nil {
		return nil, nil, skip, contentError{errors.New(refused.description)}
	}
	return first, nil, skip, nil
}

// stopOf is the Held or Deferral Result a provider's refusal of the first send means, nil when it
// means neither. The error is the one to return with it: the refusal itself, or the failure to
// remember it.
func (m *Module) stopOf(ctx context.Context, s *ent.Scoped, g gateResult, req Request, err error) (*Result, error) {
	if reason := heldBy(err); reason != "" {
		if reason == holdreason.ProviderRefused {
			// The provider refused the credentials: remember it, so every send through the
			// Integration holds until it is saved again, and none asks the provider meanwhile.
			if rerr := m.refuse(ctx, s, g.route.Integration); rerr != nil {
				return nil, rerr
			}
		}
		// The adapter found the source blocked only at the provider (email: the Sending
		// domain lost verification between prepare and the signer): a hold, not a failure.
		// The claim is dropped so the same Request can run again.
		return &Result{Outcome: Held, Hold: reason}, err
	}
	if wait := deferralBackoff(err); wait > 0 && req.Kind() != outboundmessage.KindTransactional {
		// The provider answered "too fast" or "quota exceeded": it refused the
		// message, so nothing left. A Deferral, like our own spent limit: the claim
		// is dropped, no attempt is consumed and the same Request runs again later.
		// Transactional mail never defers (ADR 0023): it falls through to the
		// retryable error, which releases the claim for the caller's retry.
		return &Result{Outcome: Deferral, Cause: deferralcause.SlowDown, Wait: wait}, err
	}
	return nil, nil
}

// dropTarget removes a target its provider said reaches no one, in a transaction of its own that
// also frees the message's place at it, and returns the Skipped reason the target now has.
func (m *Module) dropTarget(ctx context.Context, s *ent.Scoped, req Request, g gateResult, t target, messageID int64) (skipreason.Reason, error) {
	ch, err := m.channel(req.Channel())
	if err != nil {
		return "", err
	}
	var skip skipreason.Reason
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var uerr error
		if skip, uerr = ch.unreachable(ctx, ts, pub, g.route.Integration, req, t, messageID); uerr != nil {
			return uerr
		}
		// The message never reaches the target: it frees its place there (ADR 0178).
		return m.caps.Uncount(ctx, ts, frequencycap.UncountInput{MessageID: messageID, Destination: t.dest})
	})
	return skip, err
}

// refuse records that the provider refused the Integration's credentials (ProviderRefusedAt), which
// holds the Integration's Send route (holdreason.ProviderRefused).
func (m *Module) refuse(ctx context.Context, s *ent.Scoped, integ *ent.Integration) error {
	if integ == nil {
		return nil
	}
	if err := m.standing.RecordRefused(ctx, s, integ); err != nil {
		return fmt.Errorf("outbound: hold the refused integration: %w", err)
	}
	return nil
}

// accepted is the first target the provider took a message for.
type accepted struct {
	to      target
	receipt messaging.Receipt
}

// contactErased reports whether the Request carries a Contact that no longer exists in
// the Workspace. A send with no Contact (a Transactional address) is never erased here.
func (m *Module) contactErased(ctx context.Context, s *ent.Scoped, req Request) (bool, error) {
	c := req.Contact
	if c == nil || c.WorkspaceID != s.WorkspaceID() {
		// No Contact, or another Workspace's row (not "erased": the claim refuses it).
		return false, nil
	}
	exists, err := s.Contact().Query().Where(contact.ID(c.ID)).Exist(ctx)
	return !exists, err
}

// Preflight reports whether a Workspace could send content right now through the Integration of
// its Channel integrationID names (nil is that Channel's default), without sending or recording
// anything. Planning code (Broadcast) calls it to fail or pause fast instead of finding out per
// recipient; the answer is the same Send route and gate Send applies.
func (m *Module) Preflight(ctx context.Context, s *ent.Scoped, content Content, integrationID *int64) (holdreason.Reason, error) {
	if content == nil {
		return "", errors.New("outbound: content required")
	}
	g, err := m.gate(ctx, s, content, integrationID)
	if err != nil {
		return "", err
	}
	return g.hold, nil
}

// MarkFailed gives up on a pending claim for good (a queue worker calls it when the
// final retry is spent), recording cause. It is a no-op for a message that already
// reached a final status or whose claim another attempt currently holds.
func (m *Module) MarkFailed(ctx context.Context, s *ent.Scoped, key string, cause error) error {
	msg, err := m.find(ctx, s, key)
	if err != nil || msg == nil {
		return err
	}
	// Never fail a message another attempt is working on: only a claim that was released
	// (a definite provider failure) or whose lease has expired. The fence is the lease, and
	// the move is a no-op when the message is final or the lease is live.
	err = move(ctx, s, msg, []predicate.OutboundMessage{
		outboundmessage.ClaimedAtLT(m.now().Add(-m.lease)),
	}, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetFailure(cause.Error()) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}

// SourceStop says when a message's source stopped taking new messages (a cancelled Broadcast, ADR
// 0150): nil while it still takes them.
type SourceStop func(context.Context, *ent.Scoped) (*time.Time, error)

// Failure is a Broadcast recipient's send that can never go out: the recipient, why, and the
// source's stop (Request.Stopped), asked as a claim asks it.
type Failure struct {
	Recipient *ent.BroadcastRecipient
	Cause     error
	Stopped   SourceStop
}

// FailRecipient ends a Broadcast recipient's send as Failed with cause, for a recipient that
// can never go out: its Contact has no address, or its last attempt failed. Every recipient
// ends with exactly one Outbound message (ADR 0015), so this records one even when no attempt
// ever claimed the key. It is idempotent: a message already final is replayed, not rewritten
// (a send that reached the provider stays Sent).
//
// A live claim is another attempt that may still reach the provider, so nothing is failed and
// the error is ErrInProgress: the caller waits out the lease and asks again. The source's stop is
// asked as a claim asks it (Failure.Stopped): a failure recorded where no message was, and a claim
// left behind after the stop, are withdrawn, and the recipient keeps no row.
func (m *Module) FailRecipient(ctx context.Context, s *ent.Scoped, f Failure) (Result, error) {
	rec, cause := f.Recipient, f.Cause
	key := broadcastRecipientKey(rec.ID)
	msg, err := m.find(ctx, s, key)
	if err != nil {
		return Result{}, err
	}
	if msg == nil {
		ref := BroadcastRef{BroadcastID: rec.BroadcastID, RecipientID: rec.ID}
		create := s.OutboundMessage().Create().
			SetKind(ref.Kind()).
			SetIdempotencyKey(key).
			SetSendingSource(ref.mode().Source()).
			SetStatus(outboundmessage.StatusFailed).
			SetFailure(cause.Error())
		ref.stamp(create)
		if rec.ContactID != 0 {
			create.SetContactID(rec.ContactID)
		}
		id, err := create.OnConflictColumns(outboundmessage.FieldWorkspaceID, outboundmessage.FieldIdempotencyKey).DoNothing().ID(ctx)
		inserted, err := db.Inserted(err)
		switch {
		case err != nil:
			return Result{}, err
		case inserted:
			withdrawn, err := withdrawIfStopped(ctx, s, f.Stopped, id, outboundmessage.StatusEQ(outboundmessage.StatusFailed))
			if err != nil || withdrawn {
				return lo.FromPtr(withdrawnResult(withdrawn)), err
			}
			return Result{Outcome: Failed, Failure: cause.Error(), MessageID: id}, nil
		}
		// Lost the insert race to an attempt's claim: judge that row like any other.
		if msg, err = m.find(ctx, s, key); err != nil || msg == nil {
			return Result{}, err
		}
	}
	if msg.Status != outboundmessage.StatusPending {
		return replayResult(msg), nil
	}
	if m.now().Sub(msg.ClaimedAt) < m.lease {
		return Result{}, ErrInProgress
	}
	if withdrawn, err := m.withdrawLeftBehind(ctx, s, f.Stopped, msg); err != nil || withdrawn {
		return lo.FromPtr(withdrawnResult(withdrawn)), err
	}
	err = move(ctx, s, msg, []predicate.OutboundMessage{
		outboundmessage.ClaimedAtLT(m.now().Add(-m.lease)),
	}, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetFailure(cause.Error()) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return Result{}, ErrInProgress // the claim moved under us: look again later
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: Failed, Failure: cause.Error(), MessageID: msg.ID}, nil
}

// workspace loads the Workspace row the scoped client is confined to.
func (m *Module) workspace(ctx context.Context, s *ent.Scoped) (*ent.Workspace, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbound: load workspace %d: %w", s.WorkspaceID(), err)
	}
	return ws, nil
}

// gateResult is the outcome of the source-level checks plus what they resolved.
type gateResult struct {
	hold holdreason.Reason
	// route is the Send route the message goes through.
	route Route
	// as is who the message goes out as, as the route's adapter prepared it.
	as sendAs
}

// captured is the sender's own word that its messages stay inside (ADR 0058).
func (g gateResult) captured() bool { return !g.route.DeliversOutside }

// gate runs the source-level checks, in order: Workspace freeze (the reason holds names:
// deletion, suspension, Staff hold, in that priority; a Test send from the System sender passes it
// too), then the Send route of content's Channel through the Integration integrationID names (nil
// is that Channel's default) and its own hold, and the Channel's own check of content (through).
func (m *Module) gate(ctx context.Context, s *ent.Scoped, content Content, integrationID *int64) (gateResult, error) {
	hold, err := m.holds.Reason(ctx, s)
	if err != nil || hold != "" {
		return gateResult{hold: hold}, err
	}
	onChannel := content.Channel()
	route, err := m.Route(ctx, s, onChannel, integrationID)
	if err != nil {
		return gateResult{}, err
	}
	return m.through(ctx, s, route, content)
}

// through runs the checks of a send of content through a resolved Send route, after the Workspace
// freeze: the route's own hold, then its adapter's (email: a verified Sending domain for the
// effective From address).
func (m *Module) through(ctx context.Context, s *ent.Scoped, route Route, content Content) (gateResult, error) {
	if route.Held() {
		return gateResult{hold: route.Hold}, nil
	}
	as, hold, err := route.Sender.prepare(ctx, s, content)
	if err != nil || hold != "" {
		return gateResult{hold: hold}, err
	}
	return gateResult{route: route, as: as}, nil
}

func replayResult(msg *ent.OutboundMessage) Result {
	r := Result{MessageID: msg.ID, Replayed: true}
	switch msg.Status {
	case outboundmessage.StatusSent:
		r.Outcome = Sent
	case outboundmessage.StatusSkipped:
		r.Outcome, r.SkippedReason = Skipped, lo.FromPtr(msg.SkippedReason)
	default:
		r.Outcome, r.Failure = Failed, lo.FromPtr(msg.Failure)
	}
	return r
}

// contactTag is the merge tag of one of a Contact's core fields, the name every send binds it as.
type contactTag string

const (
	tagEmail     contactTag = "email"
	tagFirstName contactTag = "first_name"
	tagLastName  contactTag = "last_name"
	tagCity      contactTag = "city"
	tagCountry   contactTag = "country"
	tagLocale    contactTag = "locale"
)

// Bindings builds the Liquid merge-tag context: the Contact's custom fields first,
// then its core fields (so a custom field can never shadow email/first_name/
// last_name/city/country/locale), then the Request's own variables, which win. The country is
// its English name, which reads in a sentence where a code would not.
func Bindings(c *ent.Contact, vars map[string]any) map[string]any {
	b := map[string]any{}
	if c != nil {
		maps.Copy(b, c.CustomFields)
		b[string(tagEmail)] = lo.FromPtr(c.Email)
		b[string(tagFirstName)] = lo.FromPtr(c.FirstName)
		b[string(tagLastName)] = lo.FromPtr(c.LastName)
		b[string(tagCity)] = lo.FromPtr(c.City)
		b[string(tagCountry)] = countryName(lo.FromPtr(c.Country))
		b[string(tagLocale)] = lo.FromPtr(c.Locale)
	}
	maps.Copy(b, vars)
	return b
}

// countryName is the English name of an ISO 3166-1 alpha-2 code ("Germany" for DE), the code
// itself when it names no region, and empty for none.
func countryName(code string) string {
	if code == "" {
		return ""
	}
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	if name := display.English.Regions().Name(region); name != "" {
		return name
	}
	return code
}

// EventVariables are the merge tags of the Event a message or a call is about, under `event`: its
// action and its properties. A Rule's http Action binds the Event the Rule fired on, an
// Automation's steps the Event their Enrollment began on (#752), so both are written alike. An
// empty action and nil properties still bind `event`, so every `event.*` tag renders empty and a
// Contact's custom field named `event` never stands in for it.
func EventVariables(action string, properties map[string]any) map[string]any {
	// A System event's keys bind by their public form, as a client sees them (ADR 0142).
	return map[string]any{"event": map[string]any{"action": action, "properties": systemevent.PublicProperties(action, properties)}}
}

// EnrollmentVariables are the merge tags an Automation's steps bind besides the Contact's: `event`,
// the copy of the Event the Enrollment began on (#752); empty for a nil run or one no Event began.
func EnrollmentVariables(run *ent.AutomationRun) map[string]any {
	if run == nil {
		return EventVariables("", nil)
	}
	return EventVariables(lo.FromPtr(run.EventAction), run.EventProperties)
}

// recordSent marks the claim Sent and publishes the Channel's sent Event in one transaction, so
// the send fact reaches the Event log iff the message is recorded as sent. The provider call
// precedes this transaction, so delivery to the provider is at-least-once; the idempotency key and
// the Event DedupID make a retry safe.
func (m *Module) recordSent(ctx context.Context, s *ent.Scoped, req Request, msg *ent.OutboundMessage, g gateResult, dest string, receipt messaging.Receipt, subject string) (Result, error) {
	now := m.now()
	var integrationID int64
	if g.route.Integration != nil {
		integrationID = g.route.Integration.ID
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		// A lost fence: our claim was taken over while the provider call ran (we outlived
		// the lease). The taker owns the record; rolling back keeps it, and its
		// sent Event, the only ones.
		if err := moveOrStale(ctx, ts, msg, outboundmessage.StatusSent, func(upd *ent.OutboundMessageScopedUpdate) {
			upd.SetSentAt(now)
			// A Channel of Bindings had no Destination at the claim: it is the first that accepted.
			upd.SetDestination(dest)
			if subject != "" {
				upd.SetSubject(subject)
			}
			if g.route.Integration != nil {
				upd.SetIntegrationID(g.route.Integration.ID)
			}
			if receipt.MessageID != "" {
				upd.SetProviderMessageID(receipt.MessageID)
			}
			if g.captured() {
				upd.SetCaptured(true)
			}
		}); err != nil {
			return err
		}
		return g.route.Sender.publishSent(ctx, pub, sentFact{
			observedAt:    now,
			workspaceID:   s.WorkspaceID(),
			req:           req,
			messageID:     msg.ID,
			dest:          dest,
			as:            g.as,
			receipt:       receipt,
			captured:      g.captured(),
			integrationID: integrationID,
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: Sent, MessageID: msg.ID}, nil
}

// ListTransactional returns one page of the Workspace's Transactional Outbound messages,
// most recent first.
func (m *Module) ListTransactional(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.OutboundMessage], error) {
	return pagination.List(ctx, p, s.OutboundMessage().Query().
		Where(outboundmessage.KindEQ(outboundmessage.KindTransactional)).
		Order(ent.Desc(outboundmessage.FieldID)))
}

// recordOptedOut records that the person at target stopped at the provider, which refused the send
// for it: the Channel's opt-out (channel.optedOut), in a transaction of its own that also frees the
// message's place at it.
func (m *Module) recordOptedOut(ctx context.Context, s *ent.Scoped, req Request, g gateResult, msg *ent.OutboundMessage, t target) error {
	ch, err := m.channel(req.Channel())
	if err != nil {
		return err
	}
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		if err := ch.optedOut(ctx, ts, pub, g.route.Integration, msg, t, m.now()); err != nil {
			return err
		}
		// The message never reaches the target: it frees its place there (ADR 0178).
		return m.caps.Uncount(ctx, ts, frequencycap.UncountInput{MessageID: msg.ID, Destination: t.dest})
	})
}

// ErrKeyReused is a request whose key an earlier send already holds for a different request: the
// caller reused its Idempotency-Key. Replaying the earlier send would answer a request that was
// never made, so it is refused.
var ErrKeyReused = errors.New("outbound: the idempotency key was used for a different request")

// sameRequest refuses req when the send recorded under its key (existing, nil for none) was made
// for a different request. A send without a digest (every surface but a Transactional one with a
// client key, and a message whose digest Erasure cleared) is not compared.
func sameRequest(existing *ent.OutboundMessage, req Request) error {
	if existing == nil || existing.RequestDigest == nil || req.digest == "" || *existing.RequestDigest == req.digest {
		return nil
	}
	return ErrKeyReused
}
