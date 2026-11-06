package sendingdomains

import (
	"context"
	"net"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/sending"
)

// RecordDKIM stores what a live DKIM check of dom found (ADR 0010): last_checked_at, verified_at
// when it becomes verified (clearing the loss) and verification_lost_at when it stops being. dom is
// the kept domain as the check read it before its lookup. The write is a compare-and-swap on the
// state the check read and lands only while the domain is still kept: a check that lost to another
// one, or a domain deleted during the lookup, stores nothing, and the stored state is the answer.
// It returns the domain's verified state after the write, and lost, whether this write took the
// verification away, so the Owners are told once per loss (ADR 0046). A check's outcome is not an
// edit by whoever asked for it: the write is left out of the Audit log (events.Unaudited), as is
// RecordDMARC's.
func RecordDKIM(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain, verified bool, now time.Time) (state, lost bool, err error) {
	was := dkimstate.Is(dom)
	upd := events.Unaudited(s).SendingDomain().UpdateOneID(dom.ID).
		Where(sendingdomain.DiscardedAtIsNil()).
		SetLastCheckedAt(now)
	if verified != was {
		// Compare-and-swap on the state the check read: the write lands only while the row
		// still is in it, so the Owners are told once per loss.
		if was {
			upd.Where(dkimstate.Verified()).SetVerificationLostAt(now)
		} else {
			upd.Where(sendingdomain.Not(dkimstate.Verified())).SetVerifiedAt(now).ClearVerificationLostAt()
		}
	}
	if err := upd.Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			// Lost the swap (or the domain is deleted or gone): the stored state is the answer.
			current, gerr := s.SendingDomain().Get(ctx, dom.ID)
			if gerr != nil {
				return false, false, gerr
			}
			return dkimstate.Is(current), false, nil
		}
		return false, false, err
	}
	return verified, was && !verified, nil
}

// RecordDMARC stores what a DMARC lookup of dom found (ADR 0012): dmarc_checked_at that a lookup
// answered, and dmarc_found_at when the record was first found, set when a lookup finds it and it
// was not yet, cleared when one finds none. dom is the kept domain as the check read it. The move
// of dmarc_found_at is a compare-and-swap on the state it leaves; the write is an update of its
// own, so it never touches the DKIM moments, and a check that loses the swap, or a domain deleted
// meanwhile, leaves the answer the stored row has and is no error.
func RecordDMARC(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain, found bool, now time.Time) error {
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

// Due returns up to limit ids of the Sending domains of every Workspace whose DKIM is to be
// re-checked, least-recently-checked first with NULLS FIRST: a freshly added, never-checked domain
// is verified soonest rather than starved behind already-checked ones (plain ASC sorts NULLs last
// in Postgres). A deleted (discarded) domain is skipped (ADR 0099). It reads across Workspaces,
// before any scope exists, so it takes the raw client.
func Due(ctx context.Context, client *ent.Client, limit int) ([]int64, error) {
	return client.SendingDomain.Query().
		Where(sendingdomain.DiscardedAtIsNil()).
		Order(sendingdomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		IDs(ctx)
}

// recordDomain is the domain a proof record's name belongs to: a DKIM record
// (<selector>._domainkey.<domain>.), a DMARC record (_dmarc.<domain>.) or a Join domain's
// (_sphericon-join.<domain>.).
func recordDomain(name string) (string, bool) {
	if joined, ok := strings.CutPrefix(name, joindomains.RecordLabel); ok {
		return strings.TrimSuffix(joined, "."), true
	}
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

// DevTXTLookup is the sending-domain DKIM resolver of the end-to-end suite: instead of
// querying real DNS (which no test domain publishes), it echoes back each domain's own
// stored public key, so any sending domain a scenario adds self-verifies and the send
// gate (ADR 0010) doesn't block the Mailpit loop.
// It answers a Join domain's proof record the same way: the name
// _sphericon-join.<domain> resolves to the tokens every Workspace's claim of the domain
// stores (joindomains.Tokens), so a domain an Owner adds in the members settings verifies without
// DNS. Because it answers every Workspace's token, two Workspaces claiming one domain both verify,
// so a hand-over (the loser of a take-over) can never fire here.
// A Sending domain's DMARC name answers the suggested record, so every such domain is bulk-ready.
func DevTXTLookup(client *ent.Client) domainname.TXTLookup {
	return func(ctx context.Context, name string) ([]string, error) {
		domain, ok := recordDomain(name)
		if !ok {
			return nil, &net.DNSError{IsNotFound: true, Name: name}
		}
		if strings.HasPrefix(name, joindomains.RecordLabel) {
			tokens, err := joindomains.Tokens(ctx, client, domain)
			if err != nil {
				return nil, err
			}
			if len(tokens) == 0 {
				return nil, &net.DNSError{IsNotFound: true, Name: name}
			}
			return tokens, nil
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
		if strings.HasPrefix(name, dmarcLabel) {
			_, record := sending.DMARCRecord(row.Domain)
			return []string{record}, nil
		}
		return []string{row.DkimPublicKey}, nil
	}
}
