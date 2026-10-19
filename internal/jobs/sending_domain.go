package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/sending"
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
	_, flipped, err := VerifySendingDomainByID(ctx, w.ent, w.lookup, job.Args.SendingDomainID)
	if err != nil {
		return err
	}
	if flipped {
		// Best-effort: a failed alert must not fail (and retry) the verification.
		if nerr := NotifySendingDomainUnverified(ctx, w.ent, w.sender, job.Args.SendingDomainID); nerr != nil {
			slog.WarnContext(ctx, "notify sending domain unverified failed",
				"sending_domain_id", job.Args.SendingDomainID, "err", nerr)
		}
	}
	return VerifyTrackingDomainByID(ctx, w.ent, w.tracking, job.Args.SendingDomainID)
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
	rc := river.ClientFromContext[pgx.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifySendingDomainArgs{SendingDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// DevTXTLookup is the sending-domain DKIM resolver for local/dev environments:
// instead of querying real DNS (which no local domain publishes), it echoes back
// each domain's own stored public key, so any sending domain a developer adds
// self-verifies and the send gate (ADR 0010) doesn't block the Mailpit loop.
// Production uses net.DefaultResolver.LookupTXT — this trust must never be wired
// outside a dev env.
func DevTXTLookup(client *ent.Client) sending.TXTLookup {
	const marker = "._domainkey."
	return func(ctx context.Context, name string) ([]string, error) {
		i := strings.Index(name, marker)
		if i < 0 {
			return nil, &net.DNSError{IsNotFound: true, Name: name}
		}
		domain := strings.TrimSuffix(name[i+len(marker):], ".")
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
// the transition the owner is notified about (ADR 0010 slice 3).
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
		upd.SetVerified(verified)
		if verified {
			upd.SetVerifiedAt(now)
		}
		if dom.Verified && !verified {
			flippedToUnverified = true
			slog.WarnContext(ctx, "sending domain DKIM verification lost",
				"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
		}
	}
	if err := upd.Exec(ctx); err != nil {
		return false, false, err
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
	subject := fmt.Sprintf("Action required: sending domain %s is no longer verified", dom.Domain)
	body := fmt.Sprintf(
		"The sending domain %q in your workspace is no longer verified: its DKIM DNS "+
			"record (%s._domainkey.%s) could not be found.\n\nEmail sent from this domain "+
			"is now blocked until you re-publish the record and re-verify the domain in "+
			"your sphericon settings.\n",
		dom.Domain, dom.DkimSelector, dom.Domain,
	)
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

// VerifyTrackingDomainByID re-checks a Sending domain's Tracking domain CNAME and stores
// the state (ADR 0038). A nil verifier (the feature is off) does nothing. A host another
// Workspace already holds leaves this claim pending and is not an error: retrying cannot
// change it.
func VerifyTrackingDomainByID(ctx context.Context, client *ent.Client, v *trackingdomain.Verifier, id int64) error {
	if v == nil {
		return nil
	}
	dom, err := client.SendingDomain.Get(ctx, id)
	if err != nil {
		return err
	}
	_, err = v.Check(ctx, client.Scoped(dom.WorkspaceID), id)
	if errors.Is(err, trackingdomain.ErrHostTaken) {
		slog.InfoContext(ctx, "tracking host held by another workspace",
			"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
		return nil
	}
	return err
}
