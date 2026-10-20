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
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/ratelimit"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
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
	// Deferral: the Integration's Send rate limit is spent (ADR 0023). Nothing is wrong
	// with the source, it is busy: no hold reason, no claim, no attempt consumed.
	// Result.Wait says when capacity returns.
	Deferral Outcome = "deferral"
	// Refused: a Test send the System sender will not make (ADR 0041). Reason is a Refuse*
	// constant; unlike a Held it does not clear by waiting.
	Refused Outcome = "refused"
)

// Backoffs for a provider reply that says it is busy (ADR 0023). The core does not
// auto-tune the Send rate limit; it only waits. A daily quota refills gradually, so
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

// Reasons an Outbound send is Held.
const (
	HoldSuspended        = "workspace_suspended"
	HoldPendingDeletion  = "workspace_pending_deletion"
	HoldStaff            = "staff_hold"
	HoldNoIntegration    = "no_integration"
	HoldUnverifiedDomain = "unverified_domain"
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
	switch reason {
	case HoldSuspended:
		return "sending is suspended for this workspace"
	case HoldPendingDeletion:
		return "this workspace is deleted and pending erasure"
	case HoldStaff:
		return "sending is on hold for this workspace"
	case HoldNoIntegration:
		return "no default email provider configured"
	case HoldUnverifiedDomain:
		return "sender domain is not a verified sending domain"
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
	Outcome Outcome
	// Reason explains Skipped (eligibility reason), Failed (cause) and Held (hold).
	Reason string
	// MessageID is the Outbound message row; zero when Held (nothing was recorded).
	MessageID int64
	// Replayed is true when the Outcome was read back from an earlier attempt.
	Replayed bool
	// From is, for a Test send that was Sent, the From address it went out from.
	From string
	// Refusal is, for a Deferral, how long until capacity returns (Wait) and, for a Test
	// send Deferred by the System sender's caps, the cap that was hit (Limit).
	ratelimit.Refusal
}

// Senders resolves a Workspace's email provider adapter (its default Integration).
// *messaging.Resolver satisfies it.
type Senders interface {
	EmailSender(ctx context.Context, s *ent.Scoped) (messaging.EmailSender, error)
}

// Freezer is an extra reason a Workspace may not send right now, consulted after the
// core suspension check (ADR 0007, 0009): the Staff hold plugs in here. It
// returns "" when the Workspace may send, otherwise a Hold* style reason. It reads
// through the Workspace's scope s, so an implementation holds no raw client.
type Freezer interface {
	Frozen(ctx context.Context, s *ent.Scoped, ws *ent.Workspace) (reason string, err error)
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
	freezers        []Freezer
	// system is the Instance's System sender; nil on an Instance with no outbound mail.
	system messaging.EmailSender
	// testBudget caps Test sends from the System sender; nil means uncapped.
	testBudget TestSendBudget
}

// Option customizes a Module.
type Option func(*Module)

// WithLease overrides DefaultLease (tests use a tiny lease to exercise takeover).
func WithLease(d time.Duration) Option { return func(m *Module) { m.lease = d } }

// WithTrackingDomains turns on Tracking domains (SPHERICON_TRACKING_CNAME_TARGET is set):
// a message from a Sending domain whose Tracking domain is serving gets its links on
// track.<domain>. Off, every link uses the Instance's host (ADR 0038).
func WithTrackingDomains() Option { return func(m *Module) { m.trackingDomains = true } }

// WithFreezers adds extra freeze reasons after the core suspension check.
func WithFreezers(f ...Freezer) Option {
	return func(m *Module) { m.freezers = append(m.freezers, f...) }
}

// WithSystemSender gives the module the Instance's System sender, which a Test send falls
// back to while the Workspace has no verified Sending domain (ADR 0041).
func WithSystemSender(s messaging.EmailSender) Option { return func(m *Module) { m.system = s } }

// WithTestSendBudget caps the Test sends made from the System sender.
func WithTestSendBudget(b TestSendBudget) Option { return func(m *Module) { m.testBudget = b } }

// AsSystem returns s acting as the `system` actor (ADR 0022), for a job or automation
// step that changes an audited entity in the normal course of work.
func (m *Module) AsSystem(s *ent.Scoped) *ent.Scoped {
	return m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
}

// New builds the module. tracker may be nil only if no marketing Request is sent.
func New(bus *events.Bus, senders Senders, tracker *tracking.Tracker, opts ...Option) *Module {
	m := &Module{bus: bus, senders: senders, tracker: tracker, lease: DefaultLease}
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

	// TrackID, when non-zero, turns on open/click tracking for this message
	// (the Broadcast recipient id the tracking token encodes).
	TrackID int64

	Ref Ref
}

func (r Request) validate() error {
	switch {
	case r.Key == "":
		return errors.New("outbound: idempotency key required")
	case eligibility.NormalizeDestination(r.Destination) == "":
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
	dest := eligibility.NormalizeDestination(req.Destination)

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
	g, err := m.gate(ctx, s, ws, req.FromEmail, req.FromName)
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

	built, err := m.compose(ctx, s, g.domain, req, dest, ws)
	if err != nil {
		return m.finish(ctx, s, msg, outboundmessage.StatusFailed, Failed, err.Error())
	}
	built.msg.From, built.msg.FromName = g.from, g.fromName
	built.msg.To = dest

	// Send rate limit: the last gate before the provider, so a message that was
	// Skipped, Held or failed to render never spends capacity.
	if wait, err := m.reserve(ctx, s, req, g.integration); err != nil {
		m.release(ctx, s, msg)
		return Result{}, err
	} else if wait > 0 {
		// Busy, not blocked: drop the claim so the retry starts clean and no attempt
		// is consumed.
		_, _ = s.OutboundMessage().Delete().Where(holds(msg)...).Exec(ctx)
		return Result{Outcome: Deferral, Wait: wait}, nil
	}

	receipt, err := g.sender.Send(ctx, built.msg)
	if err != nil {
		if errors.Is(err, messaging.ErrUnverifiedSendingDomain) {
			// The domain lost verification between our check and the signer's: a hold,
			// not a failure. The claim is dropped so the same Request can run again.
			_, _ = s.OutboundMessage().Delete().Where(holds(msg)...).Exec(ctx)
			return Result{Outcome: Held, Reason: HoldUnverifiedDomain}, nil
		}
		if wait := deferralBackoff(err); wait > 0 && req.Kind != outboundmessage.KindTransactional {
			// The provider answered "too fast" or "quota exceeded": it refused the
			// message, so nothing left. A Deferral, like our own spent limit: the claim
			// is dropped, no attempt is consumed and the same Request runs again later.
			// Transactional mail never defers (ADR 0023): it falls through to the
			// retryable error below, which releases the claim for the caller's retry.
			_, _ = s.OutboundMessage().Delete().Where(holds(msg)...).Exec(ctx)
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
// uses the Integration's configured sender), without sending or recording anything.
// Planning code (Broadcast) calls it to fail or pause fast instead of finding out
// per recipient; the answer is the same gate Send applies.
func (m *Module) Preflight(ctx context.Context, s *ent.Scoped, fromEmail string) (hold string, err error) {
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return "", err
	}
	g, err := m.gate(ctx, s, ws, fromEmail, "")
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
		outboundmessage.ClaimedAtLT(time.Now().Add(-m.lease)),
	}, outboundmessage.StatusFailed, func(u *ent.OutboundMessageScopedUpdate) { u.SetReason(cause.Error()) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}

// MarkRecipientFailed gives up the pending claim of a Broadcast recipient's send for
// good, recording cause (see MarkFailed).
func (m *Module) MarkRecipientFailed(ctx context.Context, s *ent.Scoped, recipientID int64, cause error) error {
	return m.MarkFailed(ctx, s, broadcastRecipientKey(recipientID), cause)
}

// workspace loads the Workspace row the scoped client is confined to.
func (m *Module) workspace(ctx context.Context, s *ent.Scoped) (*ent.Workspace, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbound: load workspace %d: %w", s.WorkspaceID(), err)
	}
	return ws, nil
}

// frozen is the Workspace freeze check: "" when the Workspace may send, else the Hold*
// reason. Every send, a Test send from the System sender included, passes it.
func (m *Module) frozen(ctx context.Context, s *ent.Scoped, ws *ent.Workspace) (string, error) {
	// Workspace deletion (ADR 0043) is its own Hold source, not the suspension: restoring
	// clears it and leaves a suspension in place. Checked first, so a suspended and deleted
	// Workspace reports the deletion.
	if ws.DeletedAt != nil {
		return HoldPendingDeletion, nil
	}
	if ws.SuspendedAt != nil {
		return HoldSuspended, nil
	}
	for _, f := range m.freezers {
		reason, err := f.Frozen(ctx, s, ws)
		if err != nil {
			return "", fmt.Errorf("outbound: freeze check: %w", err)
		}
		if reason != "" {
			return reason, nil
		}
	}
	return "", nil
}

// gateResult is the outcome of the source-level checks plus what they resolved.
type gateResult struct {
	hold string
	// integration is the default Integration the message goes through; nil when the
	// sender has no Integration row (a test double).
	integration *ent.Integration
	sender      messaging.EmailSender
	from        string
	fromName    string
	domain      string
}

// gate runs the source-level checks, in order: Workspace freeze, an Integration to
// send through, and a verified Sending domain for the effective From address.
func (m *Module) gate(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, fromEmail, fromName string) (gateResult, error) {
	hold, err := m.frozen(ctx, s, ws)
	if err != nil || hold != "" {
		return gateResult{hold: hold}, err
	}

	sender, err := m.senders.EmailSender(ctx, s)
	if errors.Is(err, messaging.ErrNoProvider) {
		return gateResult{hold: HoldNoIntegration}, nil
	}
	if err != nil {
		return gateResult{}, fmt.Errorf("outbound: resolve sender: %w", err)
	}

	// The effective From is the Request's, else the Integration's configured one —
	// the address that will really go on the wire, so the domain gate and the
	// Sending-domain stamp agree with the DKIM signer.
	from, name := effectiveFrom(sender, fromEmail, fromName)
	verified, err := messaging.HasVerifiedSendingDomain(ctx, s, from)
	if err != nil {
		return gateResult{}, fmt.Errorf("outbound: check sending domain: %w", err)
	}
	if !verified {
		return gateResult{hold: HoldUnverifiedDomain}, nil
	}
	integ, err := messaging.DefaultEmailIntegration(ctx, s)
	if err != nil && !errors.Is(err, messaging.ErrNoProvider) {
		return gateResult{}, fmt.Errorf("outbound: load integration: %w", err)
	}
	return gateResult{sender: sender, integration: integ, from: from, fromName: name, domain: messaging.DomainOf(from)}, nil
}

// effectiveFrom is the From the message goes out with: the given one, else the sender's
// configured default.
func effectiveFrom(sender messaging.EmailSender, from, name string) (string, string) {
	if d, ok := sender.(messaging.DefaultFromer); ok {
		df, dn := d.DefaultFrom()
		return messaging.FirstNonEmpty(from, df), messaging.FirstNonEmpty(name, dn)
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

// compose renders the content for one recipient and, for a marketing send, layers
// on the unsubscribe footer, the open/click tracking and the one-click header. Any
// error here is deterministic for this message, so the caller records it as Failed.
func (m *Module) compose(ctx context.Context, s *ent.Scoped, domain string, req Request, dest string, ws *ent.Workspace) (composed, error) {
	email, err := emailrender.RenderEmail(req.Subject, req.Body, Bindings(req.Contact, req.Variables))
	if err != nil {
		return composed{}, err
	}
	out := composed{msg: messaging.EmailMessage{Subject: email.Subject, HTML: email.HTML, Text: email.Text}}
	if req.Source == "" {
		return out, nil // Transactional: no unsubscribe, no tracking
	}

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
	if req.TrackID != 0 {
		// Click/open tracking is layered on after rendering so re-parsing the HTML
		// cannot mangle the pixel or links; Rewrite also appends the footer.
		html, err := tr.Rewrite(email.HTML, req.TrackID, unsub, ws.PostalAddress)
		if err != nil {
			return composed{}, fmt.Errorf("tracking rewrite: %w", err)
		}
		out.msg.HTML = html
	} else {
		footer, err := tr.UnsubscribeFooter(unsub, ws.PostalAddress)
		if err != nil {
			return composed{}, fmt.Errorf("unsubscribe footer: %w", err)
		}
		out.msg.HTML = email.HTML + footer
	}
	// RFC 8058 one-click header: the same source-scoped token the footer carries
	// (ADR 0012); the provider adapter covers it with the DKIM signature.
	url, err := tr.UnsubscribeURL(unsub)
	if err != nil {
		return composed{}, fmt.Errorf("unsubscribe url: %w", err)
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

// recordSent marks the claim Sent and publishes email.sent in one transaction, so
// the send fact reaches the Event log iff the message is recorded as sent. The
// provider call precedes this transaction, so delivery to the provider is
// at-least-once; the idempotency key and the Event DedupID make a retry safe.
func (m *Module) recordSent(ctx context.Context, s *ent.Scoped, req Request, msg *ent.OutboundMessage, g gateResult, dest string, receipt messaging.Receipt) (Result, error) {
	contactID := req.ContactID
	if req.Contact != nil {
		contactID = req.Contact.ID
	}
	now := time.Now()
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		// A lost fence: our claim was taken over while the provider call ran (we outlived
		// the lease). The taker owns the record; rolling back keeps it, and its
		// email.sent Event, the only ones.
		if err := moveOrStale(ctx, ts, msg, outboundmessage.StatusSent, func(upd *ent.OutboundMessageScopedUpdate) {
			upd.SetSentAt(now)
			if g.integration != nil {
				upd.SetIntegrationID(g.integration.ID)
			}
			if receipt.MessageID != "" {
				upd.SetProviderMessageID(receipt.MessageID)
			}
		}); err != nil {
			return err
		}
		return pub.Publish(ctx, &events.EmailEngagement{
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
	// Asker is the User who asks. Set, it lets a Workspace with no verified Sending domain
	// send from the System sender, to Asker.Email only (ADR 0041). Nil (an API token) keeps
	// the Sending-domain gate for every Test send.
	Asker *Asker
}

// TestSubjectPrefix marks a preview send so it is never mistaken for the real mailing.
const TestSubjectPrefix = "[Test] "

// TestReport is what a Broadcast's Test send did, worded for a human: the Result, plus
// Detail, "" when the message was sent and otherwise why it was not.
type TestReport struct {
	Result
	Detail string
}

// SendBroadcastTest sends the Broadcast to one address with sample merge data for
// an author to preview it. The report's Detail is "" when the message was sent,
// otherwise the human-readable reason it was not (a send error, a Hold, a refusal or
// content that did not render), so every surface words a refused preview the same way.
func (m *Module) SendBroadcastTest(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, to string, asker *Asker) TestReport {
	res, err := m.SendTest(ctx, s, TestRequest{
		To:        to,
		Subject:   TestSubjectPrefix + b.Subject,
		Body:      b.Body,
		Variables: map[string]any{"first_name": "Alex", "last_name": "Sample", "email": to},
		FromEmail: lo.FromPtr(b.FromEmail),
		FromName:  lo.FromPtr(b.FromName),
		Asker:     asker,
	})
	report := TestReport{Result: res}
	switch {
	case err != nil:
		report.Detail = "send failed: " + err.Error()
	case res.Outcome == Held, res.Outcome == Refused:
		report.Detail = Detail(res.Reason)
	case res.Outcome == Deferral:
		report.Detail = "too many test sends from the system sender, try again later"
	case res.Outcome == Failed: // the content did not render
		report.Detail = res.Reason
	}
	return report
}

// SendTest sends one message for an author to preview. The author names the
// address, so it skips Send-eligibility and carries no unsubscribe or tracking, and
// it records no Outbound message and no email.sent Event — but it still passes the
// Workspace freeze and the Sending-domain gate and is DKIM-signed like any send: a
// suspended Workspace sends nothing, test or not (ADR 0015). Outcomes are Sent,
// Failed (the content did not render) or Held; there is no replay.
//
// With an Asker and no verified Sending domain in the Workspace it goes from the System
// sender instead (ADR 0041): after the same freeze check, only to the Asker's own address,
// within the per-User and per-Workspace caps, and Refused when the Instance has no System
// sender. The gate stays here: no other module decides which sender a Test send uses.
func (m *Module) SendTest(ctx context.Context, s *ent.Scoped, req TestRequest) (Result, error) {
	ws, err := m.workspace(ctx, s)
	if err != nil {
		return Result{}, err
	}
	if req.Asker != nil {
		anyVerified, err := s.SendingDomain().Query().Where(sendingdomain.Verified(true)).Exist(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("outbound: check sending domains: %w", err)
		}
		if !anyVerified {
			return m.sendTestFromSystem(ctx, s, ws, req)
		}
	}
	g, err := m.gate(ctx, s, ws, req.FromEmail, req.FromName)
	if err != nil {
		return Result{}, err
	}
	if g.hold != "" {
		return Result{Outcome: Held, Reason: g.hold}, nil
	}
	to := eligibility.NormalizeDestination(req.To)
	msg, err := m.composeTest(ctx, s, ws, g.domain, req, to)
	if err != nil {
		return Result{Outcome: Failed, Reason: err.Error()}, nil
	}
	return sendTestMessage(ctx, g.sender, msg, g.from, g.fromName, to)
}

// composeTest renders a Test send's content for to: no unsubscribe, no tracking.
func (m *Module) composeTest(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, domain string, req TestRequest, to string) (messaging.EmailMessage, error) {
	built, err := m.compose(ctx, s, domain, Request{Subject: req.Subject, Body: req.Body, Variables: req.Variables}, to, ws)
	return built.msg, err
}

// sendTestMessage sends a rendered Test message from from and reports it Sent. No Outbound
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

// sendTestFromSystem is the Test send of a Workspace with no verified Sending domain.
func (m *Module) sendTestFromSystem(ctx context.Context, s *ent.Scoped, ws *ent.Workspace, req TestRequest) (Result, error) {
	hold, err := m.frozen(ctx, s, ws)
	if err != nil {
		return Result{}, err
	}
	if hold != "" {
		return Result{Outcome: Held, Reason: hold}, nil
	}
	to := eligibility.NormalizeDestination(req.To)
	if to != eligibility.NormalizeDestination(req.Asker.Email) {
		return Result{Outcome: Refused, Reason: RefuseOtherRecipient}, nil
	}
	if m.system == nil {
		return Result{Outcome: Refused, Reason: RefuseNoSystemSender}, nil
	}
	// Render before taking a slot: content that cannot render never costs the budget.
	msg, err := m.composeTest(ctx, s, ws, "", req, to)
	if err != nil {
		return Result{Outcome: Failed, Reason: err.Error()}, nil
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
