// Package domainverify is the module behind the Sending domain verification jobs (ADR 0010,
// ADR 0038): the live DKIM check and what it decides (the verified moments, the loss of
// verification, the Owners' notice of it), and the Tracking domain check and what it decides
// (a host another Workspace holds, a stale answer, a state that went back a step and its
// notice). The jobs only pick which domains are due and call it.
//
// It reads a Sending domain by id to learn its Workspace, before any scope exists, so it holds
// the raw client (the same shape as internal/joindomains). Every write goes through the
// Workspace's scope.
package domainverify

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
)

// Module checks Sending domains. tracking is nil while the Tracking domain feature is off.
type Module struct {
	client   *ent.Client
	lookup   sending.TXTLookup
	tracking *trackingdomain.Verifier
	notify   ownernotice.Notify
}

func New(client *ent.Client, lookup sending.TXTLookup, tracking *trackingdomain.Verifier, notify ownernotice.Notify) *Module {
	return &Module{client: client, lookup: lookup, tracking: tracking, notify: notify}
}

// Verify runs both checks of one Sending domain. The Tracking domain check is independent of
// DKIM: a DKIM lookup error is returned (the job retries) but never skips it.
func (m *Module) Verify(ctx context.Context, id int64) error {
	_, dkimErr := m.VerifyDKIM(ctx, id)
	trackErr := m.VerifyTracking(ctx, id)
	return errors.Join(dkimErr, trackErr)
}

// VerifyDKIM re-checks a domain's DKIM DNS and persists the result: last_checked_at,
// verified_at when it becomes verified (clearing the loss) and verification_lost_at when it
// stops being. It returns the resulting verified state. A check that loses a live domain's verification mails the Owners, best-effort:
// a failed notice never fails the check. The write is a compare-and-swap on the state the check
// read, so a check that lost to another one stores nothing and reports no flip: the Owners are
// told once per loss (ADR 0046).
func (m *Module) VerifyDKIM(ctx context.Context, id int64) (verified bool, err error) {
	dom, err := m.client.SendingDomain.Get(ctx, id)
	if err != nil {
		return false, err
	}
	verified, err = sending.VerifyDKIM(ctx, m.lookup, dom.DkimSelector, dom.Domain, dom.DkimPublicKey)
	if err != nil {
		return false, err
	}

	now := time.Now()
	was := dkimstate.Is(dom)
	upd := m.client.Scoped(dom.WorkspaceID).SendingDomain().UpdateOneID(id).SetLastCheckedAt(now)
	flipped := false
	if verified != was {
		// Compare-and-swap on the state the check read: the write lands only while the row
		// still is in it, so the Owners are told once per loss.
		if was {
			upd.Where(dkimstate.Verified()).SetVerificationLostAt(now)
		} else {
			upd.Where(sendingdomain.Not(dkimstate.Verified())).SetVerifiedAt(now).ClearVerificationLostAt()
		}
		flipped = was
	}
	if err := upd.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			// Lost the swap (or the domain is gone): the stored state is the answer.
			current, gerr := m.client.SendingDomain.Get(ctx, id)
			if gerr != nil {
				return false, gerr
			}
			return dkimstate.Is(current), nil
		}
		return false, err
	}
	if flipped {
		slog.WarnContext(ctx, "sending domain DKIM verification lost",
			"sending_domain_id", id, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
		m.notifyUnverified(ctx, dom)
	}
	return verified, nil
}

func (m *Module) notifyUnverified(ctx context.Context, dom *ent.SendingDomain) {
	data := map[string]any{"Domain": dom.Domain, "Record": dom.DkimSelector + "._domainkey." + dom.Domain}
	m.notify.TellBestEffort(ctx, dom.WorkspaceID, "email.sending_domain_unverified", data, "sending_domain_id", dom.ID)
}

// VerifyTracking re-checks a Sending domain's Tracking domain (the CNAME, then the HTTPS
// probe) and stores the state (ADR 0038). No verifier (the feature is off) does nothing. A host
// another Workspace already holds leaves this claim pending and is not an error: retrying
// cannot change it. Neither is a state another check changed first
// (lifecycle.ErrInvalidTransition): the stale answer is dropped. A host that was dns_verified
// or serving and went back a step notifies the Workspace's Owners, best-effort; every change
// publishes its domain event inside the check.
func (m *Module) VerifyTracking(ctx context.Context, id int64) error {
	if m.tracking == nil {
		return nil
	}
	dom, err := m.client.SendingDomain.Get(ctx, id)
	if err != nil {
		return err
	}
	next, err := m.tracking.Check(ctx, m.client.Scoped(dom.WorkspaceID), id)
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
	if trackingRank(next) < trackingRank(dom.TrackingState) {
		m.notifyRegressed(ctx, dom, next)
	}
	return nil
}

func (m *Module) notifyRegressed(ctx context.Context, dom *ent.SendingDomain, next sendingdomain.TrackingState) {
	data := map[string]any{
		"Host": sendingdomains.TrackingHost(dom.Domain), "Domain": dom.Domain,
		"From": dom.TrackingState, "To": next, "Target": m.tracking.Target,
	}
	m.notify.TellBestEffort(ctx, dom.WorkspaceID, "email.tracking_domain_regressed", data, "sending_domain_id", dom.ID)
}

// Due returns up to limit ids of the Sending domains whose DKIM is to be re-checked,
// least-recently-checked first with NULLS FIRST: a freshly added, never-checked domain is
// verified soonest rather than starved behind already-checked ones (plain ASC sorts NULLs
// last in Postgres). An archived domain is skipped (ADR 0049).
func (m *Module) Due(ctx context.Context, limit int) ([]int64, error) {
	return m.client.SendingDomain.Query().
		Where(sendingdomain.DiscardedAtIsNil()).
		Order(sendingdomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		IDs(ctx)
}

// TrackingDue returns every Sending domain whose Tracking domain is dns_verified or serving,
// so a host that stops answering is demoted within one tick. An archived Sending domain is
// skipped, a named exception to ADR 0049's rule that job entry points include discarded rows:
// it serves nothing to demote. There is no batch limit and no DKIM dependency: a pending
// Tracking domain is only checked when its owner clicks Verify.
func (m *Module) TrackingDue(ctx context.Context) ([]int64, error) {
	return m.client.SendingDomain.Query().
		Where(trackinghostlog.Serving()).
		IDs(ctx)
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

// ShortcutDomain is the one domain that verifies without DNS in a non-production Instance:
// a Sending domain or a Join domain on it is trusted at once, so a developer can try the
// flow without publishing a record. Every other domain is resolved through real DNS.
const ShortcutDomain = "getsphericon.io"

// ShortcutTXTLookup is the resolver of a development Instance: the proof records of
// ShortcutDomain are answered from what we stored for it (see DevTXTLookup), every other
// name goes to upstream. Production uses upstream alone: this trust must never be wired outside a
// dev env.
func ShortcutTXTLookup(client *ent.Client, upstream sending.TXTLookup) sending.TXTLookup {
	dev := DevTXTLookup(client)
	return func(ctx context.Context, name string) ([]string, error) {
		if domain, ok := recordDomain(name); ok && domain == ShortcutDomain {
			return dev(ctx, name)
		}
		return upstream(ctx, name)
	}
}

// recordDomain is the domain a proof record's name belongs to: a DKIM record
// (<selector>._domainkey.<domain>.) or a Join domain's (_sphericon-join.<domain>.).
func recordDomain(name string) (string, bool) {
	if joined, ok := strings.CutPrefix(name, joindomains.RecordLabel); ok {
		return strings.TrimSuffix(joined, "."), true
	}
	_, after, ok := strings.Cut(name, dkimMarker)
	return strings.TrimSuffix(after, "."), ok
}

const dkimMarker = "._domainkey."

// DevTXTLookup is the sending-domain DKIM resolver of the end-to-end suite: instead of
// querying real DNS (which no test domain publishes), it echoes back each domain's own
// stored public key, so any sending domain a scenario adds self-verifies and the send
// gate (ADR 0010) doesn't block the Mailpit loop.
// It answers a Join domain's proof record the same way: the name
// _sphericon-join.<domain> resolves to the tokens every Workspace's claim of the domain
// stores, so a domain an Owner adds in the members settings verifies without DNS.
// Because it answers every Workspace's token, two Workspaces claiming one domain both verify, so
// a hand-over (the loser of a take-over) can never fire here.
// A development Instance reaches it only for ShortcutDomain, through ShortcutTXTLookup.
func DevTXTLookup(client *ent.Client) sending.TXTLookup {
	return func(ctx context.Context, name string) ([]string, error) {
		domain, ok := recordDomain(name)
		if !ok {
			return nil, &net.DNSError{IsNotFound: true, Name: name}
		}
		if strings.HasPrefix(name, joindomains.RecordLabel) {
			return devJoinTokens(ctx, client, name, domain)
		}
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
