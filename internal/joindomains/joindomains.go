// Package joindomains is the module for a Workspace's Join domains: a domain an
// Owner proves control of with a DNS TXT record so that, in a later step, Users with an
// address on it can join the Workspace. /site is a thin adapter over it.
//
// It is one of the few holders of the raw *ent.Client (docs/architecture.md): a domain is verified by
// one Workspace across the Instance, so looking up who holds it, and moving it from the
// Workspace that held it to the one that now proves it, happens before and across
// Workspace scopes. Everything else goes through the caller's *ent.Scoped. The periodic
// re-check of verified domains (Due, Recheck) runs on the raw client too: a job lists the
// due domains of every Workspace and scopes to each one's Workspace only after its row is
// loaded.
package joindomains

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/ownernotice"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/validation"
)

// RecordLabel is the label the TXT record sits under: _sphericon-join.<domain>.
const RecordLabel = "_sphericon-join."

var (
	// ErrAlreadyExists means the Workspace already has this domain.
	ErrAlreadyExists = validation.Conflict("joindomains: this domain is already added", "errors.join_domain_exists")
	// ErrRecordNotFound means the TXT record is not published (or carries another value).
	ErrRecordNotFound = validation.Refuse("joindomains: the TXT record was not found", "errors.join_domain_record_missing")
	// ErrHeldElsewhere means another Workspace has the domain verified and still publishes
	// its record.
	ErrHeldElsewhere = validation.Conflict("joindomains: another workspace has verified this domain", "errors.join_domain_held")
)

// Record is the DNS TXT record an Owner publishes to prove a domain.
type Record struct {
	Host  string
	Value string
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	client *ent.Client
	bus    *events.Bus
	lookup sending.TXTLookup
	now    func() time.Time
	notify ownernotice.Notify
}

// New builds the module. lookup is the injectable TXT resolver Sending domain
// verification uses; now is the clock the check times are stamped with; notify mails a
// Workspace's Owners when it loses a verified domain.
func New(client *ent.Client, bus *events.Bus, lookup sending.TXTLookup, now func() time.Time, notify ownernotice.Notify) *Module {
	return &Module{client: client, bus: bus, lookup: lookup, now: now, notify: notify}
}

// RecordOf derives the TXT record to publish for a stored domain.
func RecordOf(d *ent.JoinDomain) Record {
	return Record{Host: RecordLabel + d.Domain, Value: d.Token}
}

// Verified reports whether the domain is currently verified.
func Verified(d *ent.JoinDomain) bool { return d.VerifiedAt != nil }

// List returns the Workspace's Join domains, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped) ([]*ent.JoinDomain, error) {
	return s.JoinDomain().Query().Order(ent.Asc(joindomain.FieldID)).All(ctx)
}

// Add stores a pending claim of the domain with a fresh token. The domain is canonicalized
// (lower case, IDN as punycode); a malformed one is a *validation.Error, one the Workspace
// already has is ErrAlreadyExists. Another Workspace's claim, pending or verified, never
// blocks it.
func (m *Module) Add(ctx context.Context, s *ent.Scoped, raw string) (*ent.JoinDomain, error) {
	domain, ok := sending.NormalizeDomain(raw)
	if !ok {
		return nil, &validation.Error{Field: "domain", Message: i18n.T("errors.join_domain_invalid", nil)}
	}
	token := "sphericon-join=" + rand.Text()

	// A scoped transaction (a savepoint), so a unique violation rolls back only this write.
	var row *ent.JoinDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var cerr error
		row, cerr = ts.JoinDomain().Create().SetDomain(domain).SetToken(token).Save(ctx)
		return cerr
	})
	if db.IsUniqueViolation(err) {
		return nil, ErrAlreadyExists
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Remove deletes the claim, and only the claim: Memberships are never touched. An unknown
// id is ent's not-found error.
func (m *Module) Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	return s.JoinDomain().DeleteOneID(id).Exec(ctx)
}

// Verify checks the TXT record live and verifies the domain when it is published. A domain
// another Workspace has verified is re-checked at the holder first: still published is
// ErrHeldElsewhere, gone moves the domain to this Workspace in one transaction, the
// holder's row becoming pending. An unpublished record is ErrRecordNotFound; a resolver
// failure is returned as is and changes nothing. A domain already verified here is returned
// as it is.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.JoinDomain, error) {
	row, err := s.JoinDomain().Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if Verified(row) {
		return row, nil
	}

	published, err := m.published(ctx, row)
	if err != nil {
		return nil, err
	}
	if !published {
		return nil, m.checked(ctx, s, row, ErrRecordNotFound)
	}

	holder, err := m.Holder(ctx, row.Domain)
	if err != nil {
		return nil, err
	}
	if holder != nil {
		held, err := m.published(ctx, holder)
		if err != nil {
			return nil, err
		}
		if held {
			return nil, m.checked(ctx, s, row, ErrHeldElsewhere)
		}
	}

	now := m.now()
	var verified *ent.JoinDomain
	markVerified := func(to *ent.Scoped) error {
		var uerr error
		verified, uerr = to.JoinDomain().UpdateOneID(row.ID).Where(joindomain.VerifiedAtIsNil()).SetVerifiedAt(now).SetLastCheckedAt(now).Save(ctx)
		return uerr
	}
	if holder == nil {
		err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error { return markVerified(ts) })
	} else {
		// The holder's row goes first: the verified-only unique index admits one at a time.
		err = m.bus.WithinHandOverTx(ctx, s, holder.WorkspaceID, func(to, from *ent.Scoped) error {
			// A miss means a re-check already reverted the holder.
			err := from.JoinDomain().UpdateOneID(holder.ID).Where(joindomain.VerifiedAtNotNil()).ClearVerifiedAt().Exec(ctx)
			if err != nil && !ent.IsNotFound(err) {
				return err
			}
			return markVerified(to)
		})
	}
	if db.IsUniqueViolation(err) {
		// Another Workspace verified it between the lookup and the write.
		return nil, ErrHeldElsewhere
	}
	if ent.IsNotFound(err) {
		// A concurrent Verify got there first: the stored row is the answer.
		return s.JoinDomain().Get(ctx, id)
	}
	if err != nil {
		return nil, err
	}
	if holder != nil {
		// After the commit: a failed mail must not undo or fail the move.
		m.notifyLost(ctx, holder)
	}
	return verified, nil
}

// Due returns up to limit ids of verified Join domains to re-check, across every
// Workspace, least-recently-checked first with never-checked ones before the rest (plain
// ascending order would put NULLs last in Postgres).
func (m *Module) Due(ctx context.Context, limit int) ([]int64, error) {
	return m.client.JoinDomain.Query().
		Where(joindomain.VerifiedAtNotNil()).
		Order(joindomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		IDs(ctx)
}

// Recheck re-reads a verified domain's TXT record. A record that is gone (or carries
// another value) reverts the domain to unverified at once, under the system actor, and
// mails the Workspace's Owners; it reports whether it did. A resolver failure is returned
// and changes nothing, so the job retries. A domain that is not verified (it changed
// since the job was queued) is left alone. Memberships are never touched.
func (m *Module) Recheck(ctx context.Context, id int64) (flipped bool, err error) {
	row, err := m.client.JoinDomain.Get(ctx, id)
	if err != nil {
		return false, err
	}
	if !Verified(row) {
		return false, nil
	}
	published, err := m.published(ctx, row)
	if err != nil {
		return false, err
	}
	s := m.client.Scoped(row.WorkspaceID)
	if published {
		return false, events.Unaudited(s).JoinDomain().UpdateOneID(id).SetLastCheckedAt(m.now()).Exec(ctx)
	}
	slog.WarnContext(ctx, "join domain verification lost",
		"join_domain_id", id, "workspace_id", row.WorkspaceID, "domain", row.Domain)
	system := m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
	err = system.JoinDomain().UpdateOneID(id).Where(joindomain.VerifiedAtNotNil()).ClearVerifiedAt().SetLastCheckedAt(m.now()).Exec(ctx)
	if ent.IsNotFound(err) {
		// Lost the swap: another re-check or a hand-over already reverted it and told the Owners.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	m.notifyLost(ctx, row)
	return true, nil
}

// notifyLost mails the Owners of the Workspace that held the domain, best-effort: a mail
// failure is logged, never returned.
func (m *Module) notifyLost(ctx context.Context, row *ent.JoinDomain) {
	data := map[string]any{"Domain": row.Domain, "Record": RecordOf(row).Host}
	m.notify.TellBestEffort(ctx, row.WorkspaceID, "email.join_domain_unverified", data, "join_domain_id", row.ID)
}

// Holder returns the Join domain row that has the domain verified, across every
// Workspace, or nil when none does. It is the Instance-wide lookup that runs before any
// Workspace scope exists.
func (m *Module) Holder(ctx context.Context, domain string) (*ent.JoinDomain, error) {
	return HolderIn(ctx, m.client, domain)
}

// HolderIn is Holder over a given client, so a transaction can read the holder it is about
// to join people to (the domain-join step of the accounts module). It reads the stored
// verification only and never looks at DNS.
func HolderIn(ctx context.Context, c *ent.Client, domain string) (*ent.JoinDomain, error) {
	row, err := c.JoinDomain.Query().
		Where(joindomain.Domain(domain), joindomain.VerifiedAtNotNil()).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	return row, err
}

// checked stamps the check time on a pending row (not a configuration change, so it stays
// out of the Audit log) and returns outcome, or the stamping error.
func (m *Module) checked(ctx context.Context, s *ent.Scoped, row *ent.JoinDomain, outcome error) error {
	err := events.Unaudited(s).JoinDomain().UpdateOneID(row.ID).SetLastCheckedAt(m.now()).Exec(ctx)
	if err != nil {
		return err
	}
	return outcome
}

// published reports whether the row's token is published at its record host. A missing
// record is (false, nil); a resolver failure is the error.
func (m *Module) published(ctx context.Context, d *ent.JoinDomain) (bool, error) {
	host := RecordOf(d).Host
	// Fully qualified: a relative name would be retried through the host's resolv.conf
	// search suffixes, which can turn a SERVFAIL into a misleading "not found".
	records, err := m.lookup(ctx, host+".")
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup %s: %w", host, err)
	}
	return slices.Contains(records, d.Token), nil
}
