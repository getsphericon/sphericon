// Package sendingdomainverification is the use case that verifies a Sending domain (ADR 0102, ADR 0010):
// the live DKIM check and what it decides (the verified moments, the loss of verification, the
// Owners' notice of it), and the DMARC lookup next to it, the bulk-ready signal of ADR 0012. It
// looks the records up and stores their outcome through the sendingdomains module, which owns the
// row (RecordDKIM, RecordDMARC), and tells the Owners of a lost verification through their notice.
// The Verify button calls Verify; the jobs pick which domains are due (sendingdomains.Due), scope
// each to its Workspace and call Recheck.
package sendingdomainverification

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

// UseCase checks Sending domains. Every call receives the Workspace-scoped client (ADR 0017) of
// the domain's Workspace, built by the entry point.
type UseCase struct {
	domains *sendingdomains.Module
	bus     *events.Bus
	lookup  domainname.TXTLookup
	now     func() time.Time
}

// New builds the domain verification use case: bus opens the transaction a DKIM outcome is stored
// and its loss published in (which tells the Owners, ADR 0105), lookup resolves the DNS records,
// and now is the Instance clock a check is stamped with.
func New(domains *sendingdomains.Module, bus *events.Bus, lookup domainname.TXTLookup, now func() time.Time) *UseCase {
	return &UseCase{domains: domains, bus: bus, lookup: lookup, now: now}
}

// Verify runs the live DNS check of the domain now, when a person asks for it, and returns the
// domain as the check left it: `verified`, `lastCheckedAt`, `dmarcCheckedAt` and `dmarcFoundAt`
// are the outcome. It confirms the domain is in the Workspace and kept first: an unknown or a
// deleted domain is sendingdomains.ErrNotFound.
func (u *UseCase) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	if _, err := sendingdomains.Editable(ctx, s, id); err != nil {
		return nil, err
	}
	if err := u.Recheck(ctx, s, id); err != nil {
		return nil, err
	}
	return u.domains.Get(ctx, s, id)
}

// Recheck checks one Sending domain: its DKIM, then its DMARC record. A DKIM lookup error is
// returned so the job retries; DMARC never fails the check nor changes its DKIM result (ADR 0012).
// A deleted domain is not kept working (ADR 0099): a check queued before the deletion does nothing.
func (u *UseCase) Recheck(ctx context.Context, s *ent.Scoped, id int64) error {
	dom, err := u.domains.Get(ctx, s, id)
	if err != nil {
		return err
	}
	if dom.DiscardedAt != nil {
		return nil
	}
	if _, err := u.verifyDKIM(ctx, s, dom); err != nil {
		return err
	}
	u.checkDMARC(ctx, s, dom)
	return nil
}

// checkDMARC looks the domain's DMARC record up and stores the answer (sendingdomains.RecordDMARC).
// A failed lookup or write is logged and changes nothing: the domain stays as the last answer left
// it.
func (u *UseCase) checkDMARC(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain) {
	found, err := sendingdomains.LookupDMARC(ctx, u.lookup, dom.Domain)
	if err != nil {
		slog.WarnContext(ctx, "sending domain DMARC lookup failed",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "domain", dom.Domain, "err", err)
		return
	}
	if err := sendingdomains.RecordDMARC(ctx, s, dom, found, u.now()); err != nil {
		slog.WarnContext(ctx, "sending domain DMARC result not stored",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "err", err)
	}
}

// VerifyDKIM re-checks a domain's DKIM DNS and persists the result (sendingdomains.RecordDKIM). It
// returns the resulting verified state. A check that loses a live domain's verification publishes
// the loss with it, which tells the Owners. A check that lost to another one stores nothing and
// reports no loss: the Owners are told once per loss (ADR 0046). A deleted
// domain is not checked: its stored state is the answer (ADR 0099).
func (u *UseCase) VerifyDKIM(ctx context.Context, s *ent.Scoped, id int64) (bool, error) {
	dom, err := u.domains.Get(ctx, s, id)
	if err != nil {
		return false, err
	}
	if dom.DiscardedAt != nil {
		return dkimstate.Is(dom), nil
	}
	return u.verifyDKIM(ctx, s, dom)
}

// verifyDKIM is VerifyDKIM of a kept domain already read. The write lands only while the domain
// is still kept, so one deleted during the lookup is neither changed nor reported.
func (u *UseCase) verifyDKIM(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain) (bool, error) {
	verified, err := sendingdomains.VerifyDKIM(ctx, u.lookup, dom.DkimSelector, dom.Domain, dom.DkimPublicKey)
	if err != nil {
		return false, err
	}
	var state, lost bool
	err = u.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		state, lost, err = sendingdomains.RecordDKIM(ctx, ts, pub, dom, verified, u.now())
		return err
	})
	if err != nil {
		return false, err
	}
	if lost {
		slog.WarnContext(ctx, "sending domain DKIM verification lost",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
	}
	return state, nil
}
