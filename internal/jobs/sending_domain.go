package jobs

import (
	"context"
	stdsql "database/sql"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
)

// recheckBatchSize bounds how many domains one periodic tick re-checks, so a
// large workspace population can't stall the tick. Domains are picked
// least-recently-checked first, so every domain is reached over successive ticks.
const recheckBatchSize = 100

// --- on-demand single-domain verification (the "Verify" button) ---

type VerifySendingDomainArgs struct {
	SendingDomainID int64 `json:"sending_domain_id"`
}

func (VerifySendingDomainArgs) Kind() string { return "sending_domain_verify" }

type VerifySendingDomainWorker struct {
	river.WorkerDefaults[VerifySendingDomainArgs]
	ent      *ent.Client
	lookup   sending.TXTLookup
	sender   messaging.EmailSender    // platform sender for the verified→unverified alert
	tracking *trackingdomain.Verifier // nil: the Tracking domain feature is off
}

func (w *VerifySendingDomainWorker) Work(ctx context.Context, job *river.Job[VerifySendingDomainArgs]) error {
	// The Tracking domain check is independent of DKIM: a DKIM lookup error is returned
	// (the job retries) but never skips it.
	_, flipped, err := VerifySendingDomainByID(ctx, w.ent, w.lookup, job.Args.SendingDomainID)
	trackErr := VerifyTrackingDomainByID(ctx, w.ent, w.tracking, w.sender, job.Args.SendingDomainID)
	if err != nil {
		return errors.Join(err, trackErr)
	}
	if flipped {
		// Best-effort: a failed alert must not fail (and retry) the verification.
		if nerr := NotifySendingDomainUnverified(ctx, w.ent, w.sender, job.Args.SendingDomainID); nerr != nil {
			slog.WarnContext(ctx, "notify sending domain unverified failed",
				"sending_domain_id", job.Args.SendingDomainID, "err", nerr)
		}
	}
	return trackErr
}

// --- periodic re-check of all domains (verified is a live property, ADR 0010) ---

type RecheckSendingDomainsArgs struct{}

func (RecheckSendingDomainsArgs) Kind() string { return "sending_domain_recheck" }

type RecheckSendingDomainsWorker struct {
	river.WorkerDefaults[RecheckSendingDomainsArgs]
	ent *ent.Client
}

// Work fans the periodic tick out into one VerifySendingDomain job per domain,
// giving each check its own isolation/retry and reusing the same path as the
// "Verify" button. Domains are picked least-recently-checked first with NULLS
// FIRST, so a freshly-added, never-checked domain (last_checked_at IS NULL) is
// verified soonest rather than starved behind already-checked ones (plain ASC
// sorts NULLs last in Postgres).
func (w *RecheckSendingDomainsWorker) Work(ctx context.Context, job *river.Job[RecheckSendingDomainsArgs]) error {
	ids, err := DomainsDueForRecheck(ctx, w.ent, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifySendingDomainArgs{SendingDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// --- periodic re-check of Tracking domains (independent of the DKIM re-check) ---

type VerifyTrackingDomainArgs struct {
	SendingDomainID int64 `json:"sending_domain_id"`
}

func (VerifyTrackingDomainArgs) Kind() string { return "tracking_domain_verify" }

type VerifyTrackingDomainWorker struct {
	river.WorkerDefaults[VerifyTrackingDomainArgs]
	ent      *ent.Client
	sender   messaging.EmailSender
	tracking *trackingdomain.Verifier // nil: the Tracking domain feature is off
}

func (w *VerifyTrackingDomainWorker) Work(ctx context.Context, job *river.Job[VerifyTrackingDomainArgs]) error {
	return VerifyTrackingDomainByID(ctx, w.ent, w.tracking, w.sender, job.Args.SendingDomainID)
}

type RecheckTrackingDomainsArgs struct{}

func (RecheckTrackingDomainsArgs) Kind() string { return "tracking_domain_recheck" }

type RecheckTrackingDomainsWorker struct {
	river.WorkerDefaults[RecheckTrackingDomainsArgs]
	ent *ent.Client
}

// Work fans the tick out into one VerifyTrackingDomain job for every Sending domain
// whose Tracking domain is dns_verified or serving, so a host that stops answering is
// demoted within one tick. There is no batch limit and no DKIM dependency: a pending
// Tracking domain is only checked when its owner clicks Verify.
func (w *RecheckTrackingDomainsWorker) Work(ctx context.Context, job *river.Job[RecheckTrackingDomainsArgs]) error {
	ids, err := w.ent.SendingDomain.Query().
		Where(sendingdomain.TrackingStateIn(sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing)).
		IDs(ctx)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifyTrackingDomainArgs{SendingDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// DevTXTLookup is the sending-domain DKIM resolver for local/dev environments:
// instead of querying real DNS (which no local domain publishes), it echoes back
// each domain's own stored public key, so any sending domain a developer adds
// self-verifies and the send gate (ADR 0010) doesn't block the Mailpit loop.
// It answers a Join domain's proof record the same way: the name
// _sphericon-join.<domain> resolves to the tokens every Workspace's claim of the domain
// stores, so a domain an Owner adds in the members settings verifies without DNS.
// Because it answers every Workspace's token, two Workspaces claiming one domain both verify, so
// a hand-over (the loser of a take-over) can never fire in dev or e2e.
// Production uses net.DefaultResolver.LookupTXT — this trust must never be wired
// outside a dev env.
func DevTXTLookup(client *ent.Client) sending.TXTLookup {
	const marker = "._domainkey."
	return func(ctx context.Context, name string) ([]string, error) {
		if joined, ok := strings.CutPrefix(name, joindomains.RecordLabel); ok {
			return devJoinTokens(ctx, client, name, strings.TrimSuffix(joined, "."))
		}
		_, after, ok := strings.Cut(name, marker)
		if !ok {
			return nil, &net.DNSError{IsNotFound: true, Name: name}
		}
		domain := strings.TrimSuffix(after, ".")
		row, err := client.SendingDomain.Query().
			Where(sendingdomain.Domain(domain)).
			First(ctx)
		if ent.IsNotFound(err) {
			return nil, &net.DNSError{IsNotFound: true, Name: name}
		}
		if err != nil {
			return nil, err
		}
		return []string{row.DkimPublicKey}, nil
	}
}

// devJoinTokens answers the proof record of a Join domain from the tokens stored for it, or
// NXDOMAIN when no Workspace claims the domain.
func devJoinTokens(ctx context.Context, client *ent.Client, name, domain string) ([]string, error) {
	rows, err := client.JoinDomain.Query().Where(joindomain.Domain(domain)).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, &net.DNSError{IsNotFound: true, Name: name}
	}
	tokens := make([]string, len(rows))
	for i, row := range rows {
		tokens[i] = row.Token
	}
	return tokens, nil
}

// DomainsDueForRecheck returns up to limit sending-domain ids to re-verify,
// least-recently-checked first with NULLS FIRST — never-checked domains come
// before already-checked ones. Extracted (and pure of the queue) so the ordering
// is testable without a river runtime.
func DomainsDueForRecheck(ctx context.Context, client *ent.Client, limit int) ([]int64, error) {
	return client.SendingDomain.Query().
		Order(sendingdomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		IDs(ctx)
}

// VerifySendingDomainByID re-checks a domain's DKIM DNS and persists the result:
// verified, last_checked_at, and (when it becomes verified) verified_at. It is
// pure of the queue so it can be tested directly, and returns the resulting
// verified state plus whether this check flipped a live domain to unverified —
// the transition the owner is notified about (ADR 0010 slice 3). A check whose flip lost
// the compare-and-swap to another check stores nothing and reports no flip.
//
// The flip self-clears (the same write persists verified=false), so the next tick
// computes flipped=false and the owner is alerted exactly once per loss.
func VerifySendingDomainByID(ctx context.Context, client *ent.Client, lookup sending.TXTLookup, id int64) (verified, flippedToUnverified bool, err error) {
	dom, err := client.SendingDomain.Get(ctx, id)
	if err != nil {
		return false, false, err
	}

	verified, err = sending.VerifyDKIM(ctx, lookup, dom.DkimSelector, dom.Domain, dom.DkimPublicKey)
	if err != nil {
		return false, false, err
	}

	now := time.Now()
	upd := client.Scoped(dom.WorkspaceID).SendingDomain().UpdateOneID(id).SetLastCheckedAt(now)
	if verified != dom.Verified {
		// The flip is a compare-and-swap on the state this check read: a check that lost to
		// another one stores nothing and reports no flip, so the owner is told once (ADR 0046).
		upd.Where(sendingdomain.Verified(dom.Verified)).SetVerified(verified)
		if verified {
			upd.SetVerifiedAt(now)
		}
		flippedToUnverified = dom.Verified && !verified
	}
	if err := upd.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			// Lost the swap (or the domain is gone): the stored state is the answer.
			current, gerr := client.SendingDomain.Get(ctx, id)
			if gerr != nil {
				return false, false, gerr
			}
			return current.Verified, false, nil
		}
		return false, false, err
	}
	if flippedToUnverified {
		slog.WarnContext(ctx, "sending domain DKIM verification lost",
			"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
	}
	return verified, flippedToUnverified, nil
}

// NotifySendingDomainUnverified emails the workspace owner(s) that a sending
// domain lost DKIM verification and its sends are now blocked (ADR 0010 slice 3,
// mirroring the workspace-suspension notify intent). Best-effort and idempotent
// per flip: the caller invokes it only on the verified→unverified transition.
// A nil sender (no platform sender configured, e.g. some tests) is a no-op.
func NotifySendingDomainUnverified(ctx context.Context, client *ent.Client, sender messaging.EmailSender, id int64) error {
	if sender == nil {
		return nil
	}
	dom, err := client.SendingDomain.Get(ctx, id)
	if err != nil {
		return err
	}
	owners, err := client.Scoped(dom.WorkspaceID).Membership().Query().
		Where(membership.RoleEQ(membership.RoleOwner)).
		WithUser().
		All(ctx)
	if err != nil {
		return err
	}
	data := map[string]any{"Domain": dom.Domain, "Record": dom.DkimSelector + "._domainkey." + dom.Domain}
	subject := i18n.T("email.sending_domain_unverified.subject", data)
	body := i18n.T("email.sending_domain_unverified.body", data)
	var errs []error
	for _, m := range owners {
		u := m.Edges.User
		if u == nil || u.Email == "" {
			continue
		}
		if _, serr := sender.Send(ctx, messaging.EmailMessage{To: u.Email, Subject: subject, Text: body}); serr != nil {
			errs = append(errs, serr)
		}
	}
	return errors.Join(errs...)
}

// VerifyTrackingDomainByID re-checks a Sending domain's Tracking domain (the CNAME, then
// the HTTPS probe) and stores the state (ADR 0038). A nil verifier (the feature is off)
// does nothing. A host another Workspace already holds leaves this claim pending and is
// not an error: retrying cannot change it. Neither is a state another check changed first
// (lifecycle.ErrInvalidTransition): the stale answer is dropped. A host that was dns_verified or serving and
// went back a step notifies the Workspace's Owners, best-effort; every change publishes
// its domain event inside the Check.
func VerifyTrackingDomainByID(ctx context.Context, client *ent.Client, v *trackingdomain.Verifier, sender messaging.EmailSender, id int64) error {
	if v == nil {
		return nil
	}
	dom, err := client.SendingDomain.Get(ctx, id)
	if err != nil {
		return err
	}
	next, err := v.Check(ctx, client.Scoped(dom.WorkspaceID), id)
	if errors.Is(err, trackingdomain.ErrHostTaken) {
		slog.InfoContext(ctx, "tracking host held by another workspace",
			"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
		return nil
	}
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		// Another check moved the row after this one read it: this answer is stale, and
		// the check that won stored a fresher one. Retrying would only repeat the lookup.
		slog.InfoContext(ctx, "tracking state changed during the check",
			"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
		return nil
	}
	if err != nil {
		return err
	}
	if sender != nil && trackingRank(next) < trackingRank(dom.TrackingState) {
		host := sendingdomains.TrackingHost(dom.Domain)
		data := map[string]any{"Host": host, "Domain": dom.Domain, "From": dom.TrackingState, "To": next, "Target": v.Target}
		subject := i18n.T("email.tracking_domain_regressed.subject", data)
		body := i18n.T("email.tracking_domain_regressed.body", data)
		if nerr := NotifyWorkspaceOwners(ctx, client, sender, dom.WorkspaceID, subject, body); nerr != nil {
			slog.WarnContext(ctx, "notify tracking domain regression failed", "sending_domain_id", id, "err", nerr)
		}
	}
	return nil
}

// trackingRank orders the states from least to most verified.
func trackingRank(s sendingdomain.TrackingState) int {
	switch s {
	case sendingdomain.TrackingStateServing:
		return 2
	case sendingdomain.TrackingStateDNSVerified:
		return 1
	default:
		return 0
	}
}

// OwnerNotifier is the joindomains.Notify that mails a Workspace's Owners through the
// system sender (a nil sender sends nothing).
func OwnerNotifier(client *ent.Client, sender messaging.EmailSender) joindomains.Notify {
	if sender == nil {
		return nil
	}
	return func(ctx context.Context, workspaceID int64, subject, body string) error {
		return NotifyWorkspaceOwners(ctx, client, sender, workspaceID, subject, body)
	}
}

// --- periodic re-check of verified Join domains (the same shape as the Sending domain one) ---

type VerifyJoinDomainArgs struct {
	JoinDomainID int64 `json:"join_domain_id"`
}

func (VerifyJoinDomainArgs) Kind() string { return "join_domain_verify" }

type VerifyJoinDomainWorker struct {
	river.WorkerDefaults[VerifyJoinDomainArgs]
	mod *joindomains.Module
}

// Work re-checks one domain. The Owners' mail is sent by the module, best-effort.
func (w *VerifyJoinDomainWorker) Work(ctx context.Context, job *river.Job[VerifyJoinDomainArgs]) error {
	_, err := w.mod.Recheck(ctx, job.Args.JoinDomainID)
	return err
}

type RecheckJoinDomainsArgs struct{}

func (RecheckJoinDomainsArgs) Kind() string { return "join_domain_recheck" }

type RecheckJoinDomainsWorker struct {
	river.WorkerDefaults[RecheckJoinDomainsArgs]
	mod *joindomains.Module
}

// Work fans the tick out into one VerifyJoinDomain job per due domain, up to
// recheckBatchSize, least-recently-checked first.
func (w *RecheckJoinDomainsWorker) Work(ctx context.Context, job *river.Job[RecheckJoinDomainsArgs]) error {
	ids, err := w.mod.Due(ctx, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifyJoinDomainArgs{JoinDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}
