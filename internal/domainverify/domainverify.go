// Package domainverify is the module behind the Sending domain verification jobs (ADR 0010): the
// live DKIM check and what it decides (the verified moments, the loss of verification, the
// Owners' notice of it). The jobs only pick which domains are due and call it.
//
// It reads a Sending domain by id to learn its Workspace, before any scope exists, so it holds
// the raw client (the same shape as internal/joindomains). Every write goes through the
// Workspace's scope.
package domainverify

import (
	"context"
	"log/slog"
	"net"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/sending"
)

// Module checks Sending domains.
type Module struct {
	client *ent.Client
	lookup sending.TXTLookup
	notify ownernotice.Notify
}

func New(client *ent.Client, lookup sending.TXTLookup, notify ownernotice.Notify) *Module {
	return &Module{client: client, lookup: lookup, notify: notify}
}

// Verify checks one Sending domain's DKIM; a lookup error is returned so the job retries.
func (m *Module) Verify(ctx context.Context, id int64) error {
	_, err := m.VerifyDKIM(ctx, id)
	return err
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
	m.notify.TellBestEffort(ctx, dom.WorkspaceID, ownernotice.Notice{Subject: i18n.EmailSendingDomainUnverifiedSubject, Body: i18n.EmailSendingDomainUnverifiedBody}, data, "sending_domain_id", dom.ID)
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
