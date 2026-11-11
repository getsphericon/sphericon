package sendingdomains

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// Verify runs the live DNS check of the domain now, when a person asks for it, and returns the
// domain as the check left it: `verified`, `lastCheckedAt`, `dmarcCheckedAt` and `dmarcFoundAt`
// are the outcome. It confirms the domain is in the Workspace and kept first: an unknown or a
// deleted domain is ErrNotFound.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	if _, err := Editable(ctx, s, id); err != nil {
		return nil, err
	}
	if err := m.Recheck(ctx, s, id); err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Recheck checks one Sending domain: its DKIM, then its DMARC record. A DKIM lookup error is
// returned so the job retries; DMARC never fails the check nor changes its DKIM result (ADR 0012).
// A deleted domain is not kept working (ADR 0099): a check queued before the deletion does nothing.
// An unknown id is ErrNotFound.
func (m *Module) Recheck(ctx context.Context, s *ent.Scoped, id int64) error {
	dom, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	if dom.DiscardedAt != nil {
		return nil
	}
	if err := m.checkDKIM(ctx, s, dom); err != nil {
		return err
	}
	m.checkDMARC(ctx, s, dom)
	return nil
}

// checkDKIM looks the DKIM record of a kept domain already read up and stores the outcome
// (recordDKIM). The write lands only while the domain is still kept, so one deleted during the
// lookup is neither changed nor reported.
func (m *Module) checkDKIM(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain) error {
	verified, err := VerifyDKIM(ctx, m.lookup, dom.DkimSelector, dom.Domain, dom.DkimPublicKey)
	if err != nil {
		return err
	}
	var lost bool
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		var err error
		lost, err = recordDKIM(ctx, ts, pub, dom, verified, m.now())
		return err
	})
	if err != nil {
		return err
	}
	if lost {
		slog.WarnContext(ctx, "sending domain DKIM verification lost",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "domain", dom.Domain)
	}
	return nil
}

// checkDMARC looks the domain's DMARC record up and stores the answer (recordDMARC). A failed
// lookup or write is logged and changes nothing: the domain stays as the last answer left it.
func (m *Module) checkDMARC(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain) {
	found, err := LookupDMARC(ctx, m.lookup, dom.Domain)
	if err != nil {
		slog.WarnContext(ctx, "sending domain DMARC lookup failed",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "domain", dom.Domain, "err", err)
		return
	}
	if err := recordDMARC(ctx, s, dom, found, m.now()); err != nil {
		slog.WarnContext(ctx, "sending domain DMARC result not stored",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "err", err)
	}
}

// recordDKIM stores what a live DKIM check of dom found (ADR 0010): last_checked_at, verified_at
// when it becomes verified (clearing the loss) and verification_lost_at when it stops being. dom is
// the kept domain as the check read it before its lookup. The write is a compare-and-swap on the
// state the check read and lands only while the domain is still kept: a check that lost to another
// one, or a domain deleted during the lookup, stores nothing, and the stored state is the answer.
// It returns lost, whether this write took the verification away; a loss publishes
// events.SendingDomainVerificationLost on pub, so the Owners are told once per loss (ADR 0046,
// ADR 0105): s and pub are the transaction's. A check's outcome is not an edit by whoever asked
// for it: the write is left out of the Audit log (events.Unaudited), as is recordDMARC's.
func recordDKIM(ctx context.Context, s *ent.Scoped, pub events.Publisher, dom *ent.SendingDomain, verified bool, now time.Time) (lost bool, err error) {
	s = events.Unaudited(s)
	trigger := DKIMMissing
	if verified {
		trigger = DKIMFound
	}
	moved, err := moveDKIM(ctx, s, dom, trigger, now, func(u *ent.SendingDomainScopedUpdate) { u.SetLastCheckedAt(now) })
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		// Lost the swap: another check moved the domain, its selector or key moved, or it is
		// deleted or gone. The stored state is the answer.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !moved {
		// The check found what the state says: only the check time moves, while the domain is
		// still the one the check judged.
		_, err := s.SendingDomain().Update().Where(asJudged(dom)...).SetLastCheckedAt(now).Save(ctx)
		return false, err
	}
	if DKIMStateOf(dom) != DKIMVerified {
		return false, nil
	}
	return true, pub.Publish(ctx, &events.SendingDomainVerificationLost{
		WorkspaceID: dom.WorkspaceID, SendingDomainID: dom.ID, Domain: dom.Domain,
		Record: dom.DkimSelector + "._domainkey." + dom.Domain,
	})
}

// recordDMARC stores what a DMARC lookup of dom found (ADR 0012): dmarc_checked_at that a lookup
// answered, and dmarc_found_at when the record was first found, set when a lookup finds it and it
// was not yet, cleared when one finds none. dom is the kept domain as the check read it. The move
// of dmarc_found_at is a compare-and-swap on the state it leaves; the write is an update of its
// own, so it never touches the DKIM moments, and a check that loses the swap, or a domain deleted
// meanwhile, leaves the answer the stored row has and is no error.
func recordDMARC(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain, found bool, now time.Time) error {
	upd := events.Unaudited(s).SendingDomain().UpdateOneID(dom.ID).
		Where(sendingdomain.DiscardedAtIsNil()).
		SetDmarcCheckedAt(now)
	switch {
	case found && dom.DmarcFoundAt == nil:
		upd.Where(sendingdomain.DmarcFoundAtIsNil()).SetDmarcFoundAt(now)
	case !found && dom.DmarcFoundAt != nil:
		upd.Where(sendingdomain.DmarcFoundAtNotNil()).ClearDmarcFoundAt()
	}
	if err := upd.Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return err
	}
	return nil
}

// Due returns up to limit of the Sending domains of every Workspace whose DKIM is to be
// re-checked, each with only its id and its Workspace (the check job names both),
// least-recently-checked first with NULLS FIRST: a freshly added, never-checked domain
// is verified soonest rather than starved behind already-checked ones (plain ASC sorts NULLs last
// in Postgres). A deleted (discarded) domain is skipped (ADR 0099). It reads across Workspaces,
// before any scope exists, so it takes the raw client.
func Due(ctx context.Context, client *ent.Client, limit int) ([]*ent.SendingDomain, error) {
	return client.SendingDomain.Query().
		Where(sendingdomain.DiscardedAtIsNil()).
		Order(sendingdomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		Select(sendingdomain.FieldID, sendingdomain.FieldWorkspaceID).
		All(ctx)
}

// recordDomain is the domain a proof record's name belongs to: a DKIM record
// (<selector>._domainkey.<domain>.) or a DMARC record (_dmarc.<domain>.).
func recordDomain(name string) (string, bool) {
	if domain, ok := strings.CutPrefix(name, dmarcLabel); ok {
		return strings.TrimSuffix(domain, "."), true
	}
	_, after, ok := strings.Cut(name, dkimMarker)
	return strings.TrimSuffix(after, "."), ok
}

const (
	dkimMarker = "._domainkey."
	dmarcLabel = "_dmarc."
)

// DevRecords is the Sending domains' source of the end-to-end suite's resolver (devdns): instead of
// real DNS (which no test domain publishes), a DKIM record name answers the domain's own stored
// public key, so any Sending domain a scenario adds self-verifies and the send gate (ADR 0010)
// doesn't block the Mailpit loop, and a DMARC name answers the suggested record, so every such
// domain is bulk-ready. It reads across Workspaces, before any scope exists.
func DevRecords(client *ent.Client) func(ctx context.Context, name string) ([]string, error) {
	return func(ctx context.Context, name string) ([]string, error) {
		domain, ok := recordDomain(name)
		if !ok {
			return nil, nil
		}
		row, err := client.SendingDomain.Query().
			Where(sendingdomain.Domain(domain)).
			First(ctx)
		if ent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(name, dmarcLabel) {
			_, record := DMARCRecord(row.Domain)
			return []string{record}, nil
		}
		return []string{row.DkimPublicKey}, nil
	}
}
