// Package outbound is the one place an email leaves sphericon on a Workspace's behalf
// (ADR 0015). Every surface — Broadcast, Automation step, Transactional — hands it
// a Request and gets back a typed Outcome; none re-implements any part of what lies
// between "this message is wanted" and "the provider accepted it":
//
//   - replay: an already-recorded send returns its recorded Outcome (idempotency);
//   - holds: Workspace freeze, no Integration, unverified Sending domain — reversible,
//     per-source, never consumed as a per-recipient failure;
//   - claim: the Outbound message row is taken before the provider is called, under
//     the Request's idempotency key, with a lease against racing retries;
//   - Send-eligibility, per message, fail-closed (a Skipped is final for that
//     destination);
//   - render, the unsubscribe footer and RFC 8058 headers (marketing only), DKIM
//     signing (inside the provider adapter), the provider call;
//   - the record of the result and the email.sent Event, in one transaction.
//
// The provider is the seam: messaging.EmailSender has an SMTP and an SES adapter in
// production and an in-memory one in tests.
package outbound

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/suppression"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/validation"
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
	// Deferral: the Integration's Provider limit is spent (ADR 0023). Nothing is wrong
	// with the source, it is busy: no hold reason, no claim, no attempt consumed.
	// Result.Wait says when capacity returns.
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
// reasons (suspended, pending deletion, Staff hold) are declared by the holds package.
const (
	HoldNoIntegration    = "no_integration"
	HoldUnverifiedDomain = "unverified_domain"
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

// Senders builds the email provider adapter for the Integration a Send route resolved; a nil one is
// ErrNoProvider. *messaging.Resolver satisfies it.
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
	bus     *events.Bus
	senders Senders
	tracker *tracking.Tracker
	// trackingDomains reports whether the Instance offers Tracking domains at all.
	trackingDomains bool
	lease           time.Duration
	holds           *holds.Holds
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

// WithTrackingDomains turns on Tracking domains (SPHERICON_TRACKING_CNAME_TARGET is set):
// a message from a Sending domain whose Tracking domain is serving gets its links on
// track.<domain>. Off, every link uses the Instance's host (ADR 0038).
func WithTrackingDomains() Option { return func(m *Module) { m.trackingDomains = true } }

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
	for _, o := range opts {
		o(m)
	}
	return m
}

// Ref ties a Request to the surface's own records. They are stored on the Outbound
// message as plain ids and stamped on the email.sent Event.
type Ref struct {
	BroadcastID        int64
	BroadcastRecipient int64
	AutomationID       int64
	AutomationRunID    int64
	AutomationStep     *int
	TemplateID         int64
}

// Request describes one email to send. Content is the already-bound Message
// content (a marketing surface passes its own copy, Transactional the live
// Template's current content — see ADR 0003, 0005); the module never looks it up.
type Request struct {
	Kind outboundmessage.Kind
	// Key is unique per logical send: a retry of the same send repeats it. Required.
	Key string

	// Destination is the email address. Contact, when the destination resolves to
	// one, supplies merge tags and attribution; ContactID alone attributes only.
	Destination string
	Contact     *ent.Contact
	ContactID   int64

	// Source is the unsubscribe Sending source ("broadcasts", "automation:<id>");
	// "" is a Transactional send (Suppression only, no unsubscribe).
	Source string

	Subject string
	Body    string // HTML
	// Variables are extra merge tags; they win over Contact fields.
	Variables map[string]any

	// FromEmail/FromName override the Integration's configured sender.
	FromEmail string
	FromName  string

	// IntegrationID is the Integration the message goes through; nil is the Workspace's
	// default. Its Send route decides: one that cannot send holds the message
	// (no_integration), never falling back to the default.
	IntegrationID *int64

	// Track turns on open/click tracking for this message: the tracking token names the
	// Outbound message (ADR 0053).
	Track bool

	Ref Ref
}

func (r Request) validate() error {
	switch {
	case r.Key == "":
		return errors.New("outbound: idempotency key required")
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
	g, err := m.gate(ctx, s, req.IntegrationID, req.FromEmail, req.FromName)
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

	// Send-eligibility, per message at send time, fail-closed: if the check cannot
	// be made the message is not sent and the attempt is retried.
	dec, err := eligibility.Check(ctx, s, eligibility.ChannelEmail, dest, req.Source)
	if err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	}
	if !dec.Eligible {
		return m.finish(ctx, s, msg, outboundmessage.StatusSkipped, Skipped, dec.Reason)
	}

	built, err := m.compose(ctx, s, g.domain, req, msg.ID, dest, ws)
	if err != nil {
		if !isContentError(err) {
			// The storage behind the content failed, not the content: retry rather than record a
			// message that could have been sent as Failed for good.
			m.release(ctx, s, msg)
			return Result{}, err
		}
		return m.finish(ctx, s, msg, outboundmessage.StatusFailed, Failed, err.Error())
	}
	built.msg.From, built.msg.FromName = g.from, g.fromName
	built.msg.To = dest
	built.msg.Origin = &messaging.Origin{OutboundMessageID: msg.ID, Surface: messaging.Surface(req.Kind), Source: req.Source}

	// Provider limit: the last gate before the provider, so a message that was
	// Skipped, Held or failed to render never spends capacity.
	if wait, err := m.reserve(ctx, s, req, g.route); err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	} else if wait > 0 {
		// Busy, not blocked: drop the claim so the retry starts clean and no attempt
		// is consumed.
		m.dropClaim(ctx, s, msg)
		return Result{Outcome: Deferral, Wait: wait}, nil
	}

	receipt, err := g.route.Sender.Send(ctx, built.msg)
	if err != nil {
		if errors.Is(err, messaging.ErrUnverifiedSendingDomain) {
			// The domain lost verification between our check and the signer's: a hold,
			// not a failure. The claim is dropped so the same Request can run again.
			m.dropClaim(ctx, s, msg)
			return Result{Outcome: Held, Reason: HoldUnverifiedDomain}, nil
		}
		if wait := deferralBackoff(err); wait > 0 && req.Kind != outboundmessage.KindTransactional {
			// The provider answered "too fast" or "quota exceeded": it refused the
			// message, so nothing left. A Deferral, like our own spent limit: the claim
			// is dropped, no attempt is consumed and the same Request runs again later.
			// Transactional mail never defers (ADR 0023): it falls through to the
			// retryable error below, which releases the claim for the caller's retry.
			m.dropClaim(ctx, s, msg)
			return Result{Outcome: Deferral, Wait: wait}, nil
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

// Preflight reports whether a Workspace could send right now from fromEmail (""
// uses the Integration's configured sender) through the Integration integrationID names (nil
// is the default), without sending or recording anything.
// Planning code (Broadcast) calls it to fail or pause fast instead of finding out
// per recipient; the answer is the same Send route and gate Send applies.
func (m *Module) Preflight(ctx context.Context, s *ent.Scoped, fromEmail string, integrationID *int64) (hold string, err error) {
	g, err := m.gate(ctx, s, integrationID, fromEmail, "")
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
	route    Route
	from     string
	fromName string
	domain   string
}

// captured is the sender's own word that its mail stays inside (ADR 0058).
func (g gateResult) captured() bool { return !g.route.DeliversOutside }

// gate runs the source-level checks, in order: Workspace freeze (the reason holds names:
// deletion, suspension, Staff hold, in that priority; a Test send from the System sender passes it
// too), then the Send route of the Integration integrationID names (nil is the default) and its
// own hold, and a verified Sending domain for the effective From address (through).
func (m *Module) gate(ctx context.Context, s *ent.Scoped, integrationID *int64, fromEmail, fromName string) (gateResult, error) {
	hold, err := m.holds.Reason(ctx, s)
	if err != nil || hold != "" {
		return gateResult{hold: hold}, err
	}
	route, err := m.Route(ctx, s, integrationID)
	if err != nil {
		return gateResult{}, err
	}
	return m.through(ctx, s, route, fromEmail, fromName)
}

// through runs the checks of a send through a resolved Send route, after the Workspace freeze: the
// route's own hold, then a verified Sending domain for the effective From address.
func (m *Module) through(ctx context.Context, s *ent.Scoped, route Route, fromEmail, fromName string) (gateResult, error) {
	if route.Held() {
		return gateResult{hold: route.Hold}, nil
	}

	// The effective From is the Request's, else the Integration's configured one —
	// the address that will really go on the wire, so the domain gate and the
	// Sending-domain stamp agree with the DKIM signer.
	from, name := effectiveFrom(route.Sender, fromEmail, fromName)
	// Only a sender that delivers outside needs a verified Sending domain: nothing a
	// sender that keeps its mail sends can reach anyone under a domain the Workspace
	// does not own (ADR 0058).
	if route.DeliversOutside {
		verified, err := messaging.HasVerifiedSendingDomain(ctx, s, from)
		if err != nil {
			return gateResult{}, fmt.Errorf("outbound: check sending domain: %w", err)
		}
		if !verified {
			return gateResult{hold: HoldUnverifiedDomain}, nil
		}
	}
	return gateResult{route: route, from: from, fromName: name, domain: messaging.DomainOf(from)}, nil
}

// effectiveFrom is the From the message goes out with: the given one, else the sender's
// configured default.
func effectiveFrom(sender messaging.EmailSender, from, name string) (address, displayName string) {
	if d, ok := sender.(messaging.DefaultFromer); ok {
		df, dn := d.DefaultFrom()
		return cmp.Or(from, df), cmp.Or(name, dn)
	}
	return from, name
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

// composed is a rendered message ready for the provider (From/To are added by Send).
type composed struct {
	msg messaging.EmailMessage
}

// contentError marks a compose failure that comes from the message itself (a template that does
// not render, a link that cannot be rewritten): it is the same on every attempt, so the send ends
// as Failed. Any other compose error is a failure of what the compose reads (the tracking host
// lookup), which a retry may get past.
type contentError struct{ err error }

func (e contentError) Error() string { return e.err.Error() }
func (e contentError) Unwrap() error { return e.err }

func isContentError(err error) bool {
	var ce contentError
	return errors.As(err, &ce)
}

// compose renders the content for one recipient and, for a marketing send, layers
// on the unsubscribe footer, the open/click tracking and the one-click header. A
// contentError is deterministic for this message, so the caller records it as Failed;
// any other error is not the message's and is returned to be retried.
func (m *Module) compose(ctx context.Context, s *ent.Scoped, domain string, req Request, messageID int64, dest string, ws *ent.Workspace) (composed, error) {
	email, err := emailrender.RenderEmail(req.Subject, req.Body, Bindings(req.Contact, req.Variables))
	if err != nil {
		return composed{}, contentError{err}
	}
	out := composed{msg: messaging.EmailMessage{Subject: email.Subject, HTML: email.HTML, Text: email.Text}}
	if req.Source == "" && !req.Track {
		return out, nil // Transactional, untracked: no unsubscribe, no tracking
	}
	out, err = m.layerTracking(ctx, s, domain, req, messageID, dest, ws, out)
	if err != nil {
		return composed{}, err
	}
	// The text part says what the HTML says, tracked links and footer included (ADR 0056).
	out.msg.Text = emailrender.FinalHTMLToText(out.msg.HTML)
	return out, nil
}

// layerTracking adds to a rendered message what its send needs on top: for a tracked
// Transactional send the pixel and rewritten links; for a marketing send also the unsubscribe
// footer and the one-click header. Its errors are compose's (see contentError).
func (m *Module) layerTracking(ctx context.Context, s *ent.Scoped, domain string, req Request, messageID int64, dest string, ws *ent.Workspace, out composed) (composed, error) {
	// One choice per message: every link and the List-Unsubscribe URL share one host.
	tr := m.tracker
	if m.trackingDomains {
		host, err := sendingdomains.ServingTrackingHost(ctx, s, domain)
		if err != nil {
			return composed{}, fmt.Errorf("tracking host: %w", err)
		}
		if host != "" {
			tr = tr.OnHost(host)
		}
	}
	if req.Source == "" {
		// A tracked Transactional send: pixel and rewritten links, no footer, no header (ADR 0053).
		html, err := tr.Instrument(out.msg.HTML, messageID)
		if err != nil {
			return composed{}, contentError{fmt.Errorf("tracking rewrite: %w", err)}
		}
		out.msg.HTML = html
		return out, nil
	}

	contactID := req.ContactID
	if req.Contact != nil {
		contactID = req.Contact.ID
	}
	unsub := tracking.UnsubTarget{
		Source:      req.Source,
		Destination: dest,
		WorkspaceID: ws.ID,
		ContactID:   contactID,
		BroadcastID: req.Ref.BroadcastID,
	}
	if req.Track {
		// Click/open tracking is layered on after rendering so re-parsing the HTML
		// cannot mangle the pixel or links; Rewrite also appends the footer.
		html, err := tr.Rewrite(out.msg.HTML, messageID, unsub, ws.PostalAddress)
		if err != nil {
			return composed{}, contentError{fmt.Errorf("tracking rewrite: %w", err)}
		}
		out.msg.HTML = html
	} else {
		footer, err := tr.UnsubscribeFooter(unsub, ws.PostalAddress)
		if err != nil {
			return composed{}, contentError{fmt.Errorf("unsubscribe footer: %w", err)}
		}
		out.msg.HTML += footer
	}
	// RFC 8058 one-click header: the same source-scoped token the footer carries
	// (ADR 0012); the provider adapter covers it with the DKIM signature.
	url, err := tr.UnsubscribeURL(unsub)
	if err != nil {
		return composed{}, contentError{fmt.Errorf("unsubscribe url: %w", err)}
	}
	out.msg.ListUnsubscribeURL = url
	return out, nil
}

// Bindings builds the Liquid merge-tag context: the Contact's custom fields first,
// then its core fields (so a custom field can never shadow email/first_name/
// last_name), then the Request's own variables, which win.
func Bindings(c *ent.Contact, vars map[string]any) map[string]any {
	b := map[string]any{}
	if c != nil {
		maps.Copy(b, c.CustomFields)
		b["email"], b["first_name"], b["last_name"] = "", "", ""
		if c.Email != nil {
			b["email"] = *c.Email
		}
		if c.FirstName != nil {
			b["first_name"] = *c.FirstName
		}
		if c.LastName != nil {
			b["last_name"] = *c.LastName
		}
	}
	maps.Copy(b, vars)
	return b
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

// recordSent marks the claim Sent and publishes email.sent in one transaction, so
// the send fact reaches the Event log iff the message is recorded as sent. The
// provider call precedes this transaction, so delivery to the provider is
// at-least-once; the idempotency key and the Event DedupID make a retry safe.
func (m *Module) recordSent(ctx context.Context, s *ent.Scoped, req Request, msg *ent.OutboundMessage, g gateResult, dest string, receipt messaging.Receipt) (Result, error) {
	contactID := req.ContactID
	if req.Contact != nil {
		contactID = req.Contact.ID
	}
	now := m.now()
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		// A lost fence: our claim was taken over while the provider call ran (we outlived
		// the lease). The taker owns the record; rolling back keeps it, and its
		// email.sent Event, the only ones.
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
		if err := pub.Publish(ctx, &events.EmailEngagement{
			Action:            events.NameEmailSent,
			WorkspaceID:       s.WorkspaceID(),
			ContactID:         contactID,
			Email:             dest,
			BroadcastID:       req.Ref.BroadcastID,
			DedupID:           fmt.Sprintf("email.sent:%d", msg.ID),
			Surface:           string(req.Kind),
			SendingSource:     req.Source,
			SendingDomain:     g.domain,
			AutomationID:      req.Ref.AutomationID,
			OutboundMessageID: msg.ID,
			ProviderMessageID: receipt.MessageID,
			Captured:          g.captured(),
		}); err != nil {
			return err
		}
		if !receipt.DeliveryConfirmed {
			return nil
		}
		// The provider confirmed delivery on acceptance: the same stamp and Event a
		// Delivery notice yields, in this transaction. The fence above makes it once.
		if err := StampDelivery(ctx, ts, msg.ID, ResultDelivered, now); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.EmailDelivery{
			Action:            events.NameEmailDelivered,
			WorkspaceID:       s.WorkspaceID(),
			ContactID:         contactID,
			Email:             dest,
			ProviderMessageID: receipt.MessageID,
			SendingDomain:     g.domain,
			DedupID:           fmt.Sprintf("%s:%d/%s", events.NameEmailDelivered, msg.ID, dest),
			OutboundMessageID: msg.ID,
			Surface:           string(req.Kind),
			SendingSource:     req.Source,
			BroadcastID:       req.Ref.BroadcastID,
			AutomationID:      req.Ref.AutomationID,
			Captured:          g.captured(),
		})
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: Sent, MessageID: msg.ID}, nil
}

// TestRequest is a user-initiated test send of a message to an address the
// author names explicitly (e.g. a Broadcast preview).
type TestRequest struct {
	To        string
	Subject   string
	Body      string // HTML
	Variables map[string]any
	FromEmail string
	FromName  string
	// IntegrationID is the Integration the Test send goes through, as for a Request: nil is the
	// Workspace's default, and a named one whose Send route cannot send holds the Test send
	// (no_integration) without the System sender standing in.
	IntegrationID *int64
	// Asker is the User who asks. Set, it lets a Workspace with no verified Sending domain
	// send from the System sender, to Asker.Email only (ADR 0041). Nil (an API token) keeps
	// the Sending-domain gate for every Test send.
	Asker *Asker
}

// TestSubjectPrefix marks a preview send so it is never mistaken for the real mailing.
const TestSubjectPrefix = "[Test] "

// The refusals of a send a caller asked for and is told about at once (a Test send, a
// Transactional request), each carrying the reason worded by Detail.
var (
	// ErrHeld: the source cannot send right now (a Hold); it clears once the hold is lifted.
	ErrHeld = validation.Refuse("outbound: sending is held", "errors.sending_held")
	// ErrTestRefused: the System sender will not make this Test send (ADR 0041).
	ErrTestRefused = validation.Refuse("outbound: the test send is refused", "errors.test_send_refused")
)

// SendBroadcastTest sends the Broadcast to one address with sample merge data for an author to
// preview it. A Held or Refused Test send is ErrHeld or ErrTestRefused made With the reason worded
// by Detail, content that does not render is emailrender.ErrInvalidContent with its reason, and a
// failed send is returned as the error it is, so every surface answers a refused preview the same
// way. The System sender's cap is no error: the Result is a Deferral whose Wait and Limit say when
// capacity returns.
func (m *Module) SendBroadcastTest(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, to string, asker *Asker) (Result, error) {
	res, err := m.SendTest(ctx, s, TestRequest{
		To:        to,
		Subject:   TestSubjectPrefix + b.Subject,
		Body:      b.Body,
		Variables: map[string]any{"first_name": "Alex", "last_name": "Sample", "email": to},
		FromEmail: lo.FromPtr(b.FromEmail),
		FromName:  lo.FromPtr(b.FromName),
		// The Integration a real send of the Broadcast would go through.
		IntegrationID: b.IntegrationID,
		Asker:         asker,
	})
	if err != nil {
		return Result{}, err
	}
	switch res.Outcome {
	case Held:
		return Result{}, ErrHeld.With(Detail(res.Reason))
	case Refused:
		return Result{}, ErrTestRefused.With(Detail(res.Reason))
	case Failed: // the content did not render
		return Result{}, fmt.Errorf("%w: %s", emailrender.ErrInvalidContent, res.Reason)
	case Sent, Skipped, Deferral:
		// Sent, or the System sender's cap, which Wait and Limit describe.
	}
	return res, nil
}

// SendTest sends one message for an author to preview. The author names the
// address, so it skips Send-eligibility and carries no unsubscribe or tracking, and
// it records no Outbound message and no email.sent Event — but it still passes the
// Workspace freeze and the Sending-domain gate and is DKIM-signed like any send: a
// suspended Workspace sends nothing, test or not (ADR 0015). Outcomes are Sent,
// Failed (the content did not render) or Held; there is no replay.
//
// The Test send goes through the Send route of the Integration the request names, else of the
// Workspace's default, as a real send does (#663). With an Asker, no verified Sending domain in
// the Workspace and that route either delivering outside or held for want of a default, it goes
// from the System sender instead (ADR 0041, ADR 0058): after the same freeze check, only to the
// Asker's own address, within the per-User and per-Workspace caps, and Refused when the Instance
// has no System sender. A route that keeps its mail (a Test inbox) takes the Test send itself and
// the Result is Captured; a named Integration that cannot send holds it (no_integration). The
// route is resolved once and decides both: no other module decides which sender a Test send uses.
func (m *Module) SendTest(ctx context.Context, s *ent.Scoped, req TestRequest) (Result, error) {
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return Result{}, err
	}
	// The freeze holds a Test send whichever sender would make it.
	hold, err := m.holds.Reason(ctx, s)
	if err != nil {
		return Result{}, err
	}
	if hold != "" {
		return Result{Outcome: Held, Reason: hold}, nil
	}
	route, err := m.Route(ctx, s, req.IntegrationID)
	if err != nil {
		return Result{}, err
	}
	if req.Asker != nil {
		system, err := m.systemStandsIn(ctx, s, route, req.IntegrationID)
		if err != nil {
			return Result{}, err
		}
		if system {
			return m.sendTestFromSystem(ctx, s, ws, req)
		}
	}
	g, err := m.through(ctx, s, route, req.FromEmail, req.FromName)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{Outcome: Held, Reason: g.hold}, nil
	}
	to := suppression.NormalizeDestination(req.To)
	msg, err := m.composeTest(ctx, s, ws, g.domain, req, to)
	if err != nil {
		return failedContent(err)
	}
	res, err := sendTestMessage(ctx, g.route.Sender, msg, g.from, g.fromName, to)
	res.Captured = res.Outcome == Sent && g.captured()
	return res, err
}

// systemStandsIn reports whether the System sender makes an asked-for Test send in place of the
// route (ADR 0041, ADR 0058): only while none of the Workspace's Sending domains is verified, and
// only where the route would deliver outside or the Workspace has no default to send through. A
// route that keeps its mail takes the Test send itself, so it is captured; a named Integration that
// cannot send is no reason for the System sender, and the gate holds the Test send as it holds a
// real one.
func (m *Module) systemStandsIn(ctx context.Context, s *ent.Scoped, route Route, integrationID *int64) (bool, error) {
	anyVerified, err := s.SendingDomain().Query().Where(dkimstate.Verified()).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("outbound: check sending domains: %w", err)
	}
	if anyVerified {
		return false, nil
	}
	if route.Held() {
		return integrationID == nil, nil
	}
	return route.DeliversOutside, nil
}

// failedContent is a Test send whose compose failed: a content error is the Failed result, any other
// is returned as the error it is.
func failedContent(err error) (Result, error) {
	if isContentError(err) {
		return Result{Outcome: Failed, Reason: err.Error()}, nil
	}
	return Result{}, err
}

// composeTest renders a Test send's content for to: no unsubscribe, no tracking.
func (m *Module) composeTest(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, domain string, req TestRequest, to string) (messaging.EmailMessage, error) {
	built, err := m.compose(ctx, s, domain, Request{Subject: req.Subject, Body: req.Body, Variables: req.Variables}, 0, to, ws)
	return built.msg, err
}

// sendTestMessage sends a rendered Test message from the address from and reports it Sent. No Outbound
// message is recorded.
func sendTestMessage(ctx context.Context, sender messaging.EmailSender, msg messaging.EmailMessage, from, name, to string) (Result, error) {
	msg.From, msg.FromName, msg.To = from, name, to
	if _, err := sender.Send(ctx, msg); err != nil {
		if errors.Is(err, messaging.ErrUnverifiedSendingDomain) {
			return Result{Outcome: Held, Reason: HoldUnverifiedDomain}, nil
		}
		return Result{}, fmt.Errorf("outbound: test send to %s: %w", to, err)
	}
	return Result{Outcome: Sent, From: from}, nil
}

// sendTestFromSystem is the Test send of a Workspace with no verified Sending domain, past the
// Workspace freeze.
func (m *Module) sendTestFromSystem(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, req TestRequest) (Result, error) {
	to := suppression.NormalizeDestination(req.To)
	if to != suppression.NormalizeDestination(req.Asker.Email) {
		return Result{Outcome: Refused, Reason: RefuseOtherRecipient}, nil
	}
	if m.system == nil {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	// Render before taking a slot: content that cannot render never costs the budget.
	msg, err := m.composeTest(ctx, s, ws, "", req, to)
	if err != nil {
		return failedContent(err)
	}
	// A System sender that names no From cannot send: refuse before a slot is taken.
	from, name := effectiveFrom(m.system, "", "")
	if from == "" {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	if m.testBudget != nil {
		refusal, err := m.testBudget.TakeTestSend(ctx, req.Asker.UserID, s.WorkspaceID())
		if err != nil {
			return Result{}, fmt.Errorf("outbound: test send budget: %w", err)
		}
		if refusal.Wait > 0 {
			return Result{Outcome: Deferral, Refusal: refusal}, nil
		}
	}
	return sendTestMessage(ctx, m.system, msg, from, name, to)
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
