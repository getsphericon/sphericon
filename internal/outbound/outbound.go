// Package outbound is the one place a message leaves sphericon on a Workspace's behalf, on any
// Channel (ADR 0015, ADR 0079). Every surface — Broadcast, Automation step, Transactional — hands
// it a Request and gets back a typed Outcome; none re-implements any part of what lies between
// "this message is wanted" and "the provider accepted it". The pipeline is one, in Send:
//
//   - replay: an already-recorded send returns its recorded Outcome (idempotency);
//   - holds: Workspace freeze, then the Send route of the Request's Channel and the Channel's own
//     check (email: an unverified Sending domain) — reversible, per-source, never consumed as a
//     per-recipient failure;
//   - claim: the Outbound message row is taken before the provider is called, under
//     the Request's idempotency key, with a lease against racing retries;
//   - Send-eligibility on the Channel, per message, fail-closed (a Skipped is final for that
//     destination);
//   - render, the Provider limit and the Send cap, the provider call;
//   - the record of the result and the Channel's sent Event, in one transaction.
//
// Only a Channel's renderer and provider adapter differ (channel.go). Email's (email.go) holds
// the Sending-domain gate, the unsubscribe footer and RFC 8058 headers, the open pixel and DKIM
// signing (inside the EmailSender); a Test send and the System sender are email's (testsend.go).
package outbound

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/samber/lo"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// Outcome is what an Outbound send did. Sent, Skipped and Failed are final for the
// destination; Held and Deferral are reversible per-source Outcomes — nothing was
// consumed, the caller should try the same Request again later (a Held after its own
// delay, a Deferral after Result.Wait).
type Outcome string

const (
	// Sent: the provider accepted the message (not "delivered" — see GLOSSARY).
	Sent Outcome = "sent"
	// Skipped: Send-eligibility forbids this destination. Reason is an eligibility.Reason*.
	Skipped Outcome = "skipped"
	// Failed: this one message can never be sent (e.g. its template failed to render).
	Failed Outcome = "failed"
	// Held: the source cannot send right now. Reason is a Hold* constant.
	Held Outcome = "held"
	// Deferral: a send limit is spent — the Integration's Provider limit (ADR 0023), the
	// Workspace's Send cap (ADR 0067) or the provider's own. Nothing is wrong with the source,
	// it is busy: no hold reason, no claim, no attempt consumed. Result.Wait says when
	// capacity returns and Result.Cause which limit it waits for.
	Deferral Outcome = "deferral"
	// Refused: a Test send the System sender will not make (ADR 0041). Reason is a Refuse*
	// constant; unlike a Held it does not clear by waiting.
	Refused Outcome = "refused"
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
	switch {
	case errors.Is(err, messaging.ErrQuotaExceeded):
		return QuotaBackoff
	case errors.Is(err, messaging.ErrBusy):
		return DeferralBackoff
	default:
		return 0
	}
}

// SkipContactErased is the Skipped reason of a send whose Contact was erased after
// the send was queued (ADR 0021). Nothing is recorded for it: the message row would
// carry the erased person's address.
const SkipContactErased = "contact_erased"

// Reasons an Outbound send is Held by its own source-level checks. The Workspace-level
// reasons (suspended, pending deletion, Staff hold) are declared by the holds package, and
// email's own (HoldUnverifiedDomain) by email's adapter.
const (
	HoldNoIntegration = "no_integration"
	// HoldProviderStopped: the provider has stopped the account behind the Integration (SES
	// EnforcementStatus SHUTDOWN, ADR 0067); it lifts when a quota read finds it sending again.
	HoldProviderStopped = "provider_stopped"
)

// Reasons a Test send from the System sender is Refused (ADR 0041).
const (
	// RefuseOtherRecipient: the Workspace has no verified Sending domain, so a Test send
	// may only go to the address of the User who asks.
	RefuseOtherRecipient = "other_recipient"
	// RefuseNoSystemSender: the Instance has no outbound mail, so there is no System sender
	// (or one that names no From to send from).
	RefuseNoSystemSender = "no_system_sender"
)

// Detail words a Hold or Refuse reason for a human (an API problem detail). The single
// place the wording lives, so every surface that reports a Held or Refused outcome agrees.
func Detail(reason string) string {
	if msg := holds.Message(reason); msg != "" {
		return msg
	}
	switch reason {
	case HoldNoIntegration:
		return "no email integration can send: the one named is disabled or archived, or there is no default"
	case HoldUnverifiedDomain:
		return "sender domain is not a verified sending domain"
	case HoldProviderStopped:
		return "the email provider has stopped sending from this account"
	case RefuseOtherRecipient:
		return "until a sending domain is verified, a test send can only go to your own address"
	case RefuseNoSystemSender:
		return "outbound mail is not configured on this instance, so a test send needs a verified sending domain"
	default:
		return "sending is currently on hold for this workspace: " + reason
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
	// Refusal is, for a Deferral, how long until capacity returns (Wait) and, for a Test
	// send Deferred by the System sender's caps, the cap that was hit (Limit).
	ratelimit.Refusal

	Outcome Outcome
	// Cause is, for a Deferral of a Send, the limit it waits for; empty for every other Outcome
	// and for a Test send (its cap is Limit).
	Cause providerlimit.Cause
	// Reason explains Skipped (eligibility reason), Failed (cause) and Held (hold).
	Reason string
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

// Senders builds the EmailSender email's provider adapter sends through, for the Integration a
// Send route resolved; a nil one is ErrNoProvider. *messaging.Resolver satisfies it.
type Senders interface {
	EmailSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error)
}

// Asker is the User who asks for a Test send. Only they may receive one from the System
// sender (ADR 0041).
type Asker struct {
	UserID int64
	// Email is the User's own address.
	Email string
}

// TestSendBudget meters Test sends from the System sender, per User and per Workspace
// (ADR 0041, ADR 0025). TakeTestSend reserves one slot of each and returns the zero Refusal
// when it was granted; otherwise the Refusal says how long until a slot frees and which cap
// was hit.
type TestSendBudget interface {
	TakeTestSend(ctx context.Context, userID, workspaceID int64) (ratelimit.Refusal, error)
}

// DefaultLease is how long a pending claim blocks other attempts before a retry may
// take it over.
const DefaultLease = 5 * time.Minute

// Module is the Outbound send module.
type Module struct {
	bus *events.Bus
	// channels are the Channels a message can be sent on, each with its renderer and adapter.
	channels map[messaging.Channel]channel
	senders  Senders
	tracker  *tracking.Tracker
	// trackingHosts names the Workspace's serving Tracking domain; nil while the Instance offers
	// none.
	trackingHosts TrackingHosts
	// sendCaps is whether the Instance has Send caps at all (ADR 0067); without them a
	// Workspace's stored Send cap is ignored.
	sendCaps providerlimit.SendCaps
	lease    time.Duration
	holds    *holds.Holds
	// system is the Instance's System sender; nil on an Instance with no outbound mail.
	system messaging.EmailSender
	// testBudget caps Test sends from the System sender; nil means uncapped.
	testBudget TestSendBudget
	// now is the clock the Provider limit and the claim lease are read at.
	now func() time.Time
}

// Option customizes a Module.
type Option func(*Module)

// WithLease overrides DefaultLease (tests use a tiny lease to exercise takeover).
func WithLease(d time.Duration) Option { return func(m *Module) { m.lease = d } }

// TrackingHosts names the host a Workspace's links are built on: its current Tracking domain
// while it serves, "" otherwise (ADR 0071).
type TrackingHosts interface {
	ServingHost(ctx context.Context, s *ent.Scoped) (string, error)
}

// WithTrackingDomains turns on Tracking domains (SPHERICON_TRACKING_CNAME_TARGET is set): every
// tracked message of a Workspace whose Tracking domain serves gets its links on that host. Off,
// every link uses the Instance's host (ADR 0038, ADR 0071).
func WithTrackingDomains(hosts TrackingHosts) Option {
	return func(m *Module) { m.trackingHosts = hosts }
}

// WithSendCaps says whether the Instance has Send caps (ADR 0067): with them every send of a
// Workspace that has one also spends from its 24-hour bucket. Without (the default), a stored Send
// cap has no effect.
func WithSendCaps(c providerlimit.SendCaps) Option { return func(m *Module) { m.sendCaps = c } }

// WithSystemSender gives the module the Instance's System sender, which a Test send falls
// back to while the Workspace has no verified Sending domain (ADR 0041).
func WithSystemSender(s messaging.EmailSender) Option { return func(m *Module) { m.system = s } }

// WithTestSendBudget caps the Test sends made from the System sender.
func WithTestSendBudget(b TestSendBudget) Option { return func(m *Module) { m.testBudget = b } }

// WithClock replaces the wall clock the module reads: the Provider limit's token buckets, the
// claim lease and the moment a message is recorded sent. Tests move it instead of waiting.
func WithClock(now func() time.Time) Option { return func(m *Module) { m.now = now } }

// Now reads the module's clock (WithClock): the moment a Deferral's Wait is measured from, so a
// caller that records when the deferred send is due uses the same clock.
func (m *Module) Now() time.Time { return m.now() }

// AsSystem returns s acting as the `system` actor (ADR 0022), for a job or automation
// step that changes an audited entity in the normal course of work.
func (m *Module) AsSystem(s *ent.Scoped) *ent.Scoped {
	return m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
}

// New builds the module. tracker may be nil only if no marketing Request is sent.
func New(bus *events.Bus, hs *holds.Holds, senders Senders, tracker *tracking.Tracker, opts ...Option) *Module {
	m := &Module{bus: bus, holds: hs, senders: senders, tracker: tracker, lease: DefaultLease, now: time.Now}
	m.channels = map[messaging.Channel]channel{messaging.ChannelEmail: emailChannel{m: m}}
	for _, o := range opts {
		o(m)
	}
	return m
}

// channel is the part of the send of ch, an error for a Channel no message is sent on.
func (m *Module) channel(ch messaging.Channel) (channel, error) {
	c, ok := m.channels[ch]
	if !ok {
		return nil, fmt.Errorf("outbound: no message is sent on channel %q", ch)
	}
	return c, nil
}

// Ref ties a Request to the surface's own records. They are stored on the Outbound
// message as plain ids and stamped on the Channel's sent Event.
type Ref struct {
	BroadcastID        int64
	BroadcastRecipient int64
	AutomationID       int64
	AutomationRunID    int64
	AutomationStep     *int
	TemplateID         int64
}

// Request describes one message to send on one Channel. Content is the already-bound Message
// content, whose Channel is the send's (a marketing surface passes its own copy, Transactional
// the live Template's current content — see ADR 0003, 0005); the module never looks it up.
type Request struct {
	Kind outboundmessage.Kind
	// Key is unique per logical send: a retry of the same send repeats it. Required.
	Key string

	// Destination is where the message goes on its Channel (on email the address). Contact, when
	// the destination resolves to one, supplies merge tags and attribution; ContactID alone
	// attributes only.
	Destination string
	Contact     *ent.Contact
	ContactID   int64

	// Source is the unsubscribe Sending source ("broadcasts", "automation:<id>");
	// "" is a Transactional send (Suppression only, no unsubscribe).
	Source string

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

	Ref Ref
}

// Channel is the Channel the message goes out on: its content's.
func (r Request) Channel() messaging.Channel { return r.Content.Channel() }

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
	case suppression.NormalizeDestination(r.Destination) == "":
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
	if req.Source != "" && m.tracker == nil {
		return Result{}, ErrNoTracker
	}
	onChannel := req.Channel()
	ch, err := m.channel(onChannel)
	if err != nil {
		return Result{}, err
	}
	dest := suppression.NormalizeDestination(req.Destination)

	// A recorded send replays its Outcome — even while the Workspace is frozen: the
	// message already left (or was already decided), nothing new is being sent.
	existing, err := m.find(ctx, s, req.Key)
	if err != nil {
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
		return Result{Outcome: Skipped, Reason: SkipContactErased}, nil
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
		return Result{Outcome: Held, Reason: g.hold}, nil
	}

	msg, replay, err := m.claim(ctx, s, req, dest, g, existing)
	if err != nil {
		return Result{}, err
	}
	if replay != nil {
		return *replay, nil
	}

	// Send-eligibility on the Channel, per message at send time, fail-closed: if the check
	// cannot be made the message is not sent and the attempt is retried.
	eligibleOn := string(onChannel)
	dec, err := eligibility.Check(ctx, s, eligibleOn, dest, req.Source)
	if err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	}
	if !dec.Eligible {
		return m.finish(ctx, s, msg, outboundmessage.StatusSkipped, Skipped, dec.Reason)
	}

	rendered, err := ch.render(ctx, s, req, msg.ID, dest, ws)
	if err != nil {
		if !isContentError(err) {
			// The storage behind the content failed, not the content: retry rather than record a
			// message that could have been sent as Failed for good.
			m.release(ctx, s, msg)
			return Result{}, err
		}
		return m.finish(ctx, s, msg, outboundmessage.StatusFailed, Failed, err.Error())
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

	origin := messaging.Origin{OutboundMessageID: msg.ID, Surface: messaging.Surface(req.Kind), Source: req.Source}
	receipt, err := g.route.Sender.send(ctx, g.as, rendered, dest, origin)
	if err != nil {
		if reason := heldBy(err); reason != "" {
			// The adapter found the source blocked only at the provider (email: the Sending
			// domain lost verification between prepare and the signer): a hold, not a failure.
			// The claim is dropped so the same Request can run again.
			m.dropClaim(ctx, s, msg)
			return Result{Outcome: Held, Reason: reason}, nil
		}
		if wait := deferralBackoff(err); wait > 0 && req.Kind != outboundmessage.KindTransactional {
			// The provider answered "too fast" or "quota exceeded": it refused the
			// message, so nothing left. A Deferral, like our own spent limit: the claim
			// is dropped, no attempt is consumed and the same Request runs again later.
			// Transactional mail never defers (ADR 0023): it falls through to the
			// retryable error below, which releases the claim for the caller's retry.
			m.dropClaim(ctx, s, msg)
			return Result{Outcome: Deferral, Cause: providerlimit.CauseProvider, Wait: wait}, nil
		}
		m.release(ctx, s, msg)
		return Result{}, fmt.Errorf("outbound: send to %s: %w", dest, err)
	}

	return m.recordSent(ctx, s, req, msg, g, dest, receipt)
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
func (m *Module) Preflight(ctx context.Context, s *ent.Scoped, content Content, integrationID *int64) (hold string, err error) {
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
	}, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetReason(cause.Error()) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}

// FailRecipient ends a Broadcast recipient's send as Failed with cause, for a recipient that
// can never go out: its Contact has no address, or its last attempt failed. Every recipient
// ends with exactly one Outbound message (ADR 0015), so this records one even when no attempt
// ever claimed the key. It is idempotent: a message already final is replayed, not rewritten
// (a send that reached the provider stays Sent).
//
// A live claim is another attempt that may still reach the provider, so nothing is failed and
// the error is ErrInProgress: the caller waits out the lease and asks again.
func (m *Module) FailRecipient(ctx context.Context, s *ent.Scoped, rec *ent.BroadcastRecipient, cause error) (Result, error) {
	key := broadcastRecipientKey(rec.ID)
	msg, err := m.find(ctx, s, key)
	if err != nil {
		return Result{}, err
	}
	if msg == nil {
		create := s.OutboundMessage().Create().
			SetKind(outboundmessage.KindBroadcast).
			SetIdempotencyKey(key).
			SetSendingSource(eligibility.SourceBroadcasts).
			SetBroadcastID(rec.BroadcastID).
			SetBroadcastRecipientID(rec.ID).
			SetStatus(outboundmessage.StatusFailed).
			SetReason(cause.Error())
		if rec.ContactID != 0 {
			create.SetContactID(rec.ContactID)
		}
		created, err := create.Save(ctx)
		if err == nil {
			return Result{Outcome: Failed, Reason: cause.Error(), MessageID: created.ID}, nil
		}
		if !ent.IsConstraintError(err) {
			return Result{}, err
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
	err = move(ctx, s, msg, []predicate.OutboundMessage{
		outboundmessage.ClaimedAtLT(m.now().Add(-m.lease)),
	}, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetReason(cause.Error()) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return Result{}, ErrInProgress // the claim moved under us: look again later
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: Failed, Reason: cause.Error(), MessageID: msg.ID}, nil
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
	hold string
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
	r.Reason = lo.FromPtr(msg.Reason)
	switch msg.Status {
	case outboundmessage.StatusSent:
		r.Outcome = Sent
	case outboundmessage.StatusSkipped:
		r.Outcome = Skipped
	default:
		r.Outcome = Failed
	}
	return r
}

// Bindings builds the Liquid merge-tag context: the Contact's custom fields first,
// then its core fields (so a custom field can never shadow email/first_name/
// last_name/city/country/locale), then the Request's own variables, which win. The country is
// its English name, which reads in a sentence where a code would not.
func Bindings(c *ent.Contact, vars map[string]any) map[string]any {
	b := map[string]any{}
	if c != nil {
		maps.Copy(b, c.CustomFields)
		b["email"] = lo.FromPtr(c.Email)
		b["first_name"] = lo.FromPtr(c.FirstName)
		b["last_name"] = lo.FromPtr(c.LastName)
		b["city"] = lo.FromPtr(c.City)
		b["country"] = countryName(lo.FromPtr(c.Country))
		b["locale"] = lo.FromPtr(c.Locale)
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
	return map[string]any{"event": map[string]any{"action": action, "properties": properties}}
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
func (m *Module) recordSent(ctx context.Context, s *ent.Scoped, req Request, msg *ent.OutboundMessage, g gateResult, dest string, receipt messaging.Receipt) (Result, error) {
	now := m.now()
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		// A lost fence: our claim was taken over while the provider call ran (we outlived
		// the lease). The taker owns the record; rolling back keeps it, and its
		// sent Event, the only ones.
		if err := moveOrStale(ctx, ts, msg, outboundmessage.StatusSent, func(upd *ent.OutboundMessageScopedUpdate) {
			upd.SetSentAt(now)
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
		// The provider confirmed delivery on acceptance: the same stamp a Delivery notice
		// yields, in this transaction. The fence above makes it once.
		if receipt.DeliveryConfirmed {
			if err := StampDelivery(ctx, ts, msg.ID, ResultDelivered, now); err != nil {
				return err
			}
		}
		return g.route.Sender.publishSent(ctx, pub, sentFact{
			workspaceID: s.WorkspaceID(),
			req:         req,
			messageID:   msg.ID,
			dest:        dest,
			as:          g.as,
			receipt:     receipt,
			captured:    g.captured(),
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
	query := func() *ent.OutboundMessageQuery {
		return s.OutboundMessage().Query().Where(outboundmessage.KindEQ(outboundmessage.KindTransactional))
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.OutboundMessage, error) {
			return query().Order(ent.Desc(outboundmessage.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
