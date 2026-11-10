// Package joindomains is the module for a Workspace's Join domains: a domain an
// Owner proves control of with a DNS TXT record so that, in a later step, Users with an
// address on it can join the Workspace. /site is a thin adapter over it.
//
// It is one of the few holders of the raw *ent.Client (docs/architecture/workspace-scoping.md): a domain is verified by
// one Workspace across the Instance, so looking up who holds it, and moving it from the
// Workspace that held it to the one that now proves it, happens before and across
// Workspace scopes. Everything else goes through the caller's *ent.Scoped. The periodic
// re-check of verified domains (Due, Recheck) runs on the raw client too: a job lists the
// due domains of every Workspace and scopes to each one's Workspace only after its row is
// loaded. DevRecords reads every Workspace's claim of a domain for the end-to-end suite's resolver.
package joindomains

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// recordLabel is the label the TXT record sits under: _sphericon-join.<domain>.
const recordLabel = "_sphericon-join."

var (
	// ErrAlreadyExists means the Workspace already has this domain.
	ErrAlreadyExists = validation.ConflictField("domain", "joindomains: this domain is already added", i18n.ErrorsJoinDomainExists)
	// ErrInvalid is a domain that is no domain name.
	ErrInvalid = validation.RefuseField("domain", "joindomains: invalid domain", i18n.ErrorsJoinDomainInvalid)
	// ErrRecordNotFound means the TXT record is not published (or carries another value).
	ErrRecordNotFound = validation.Refuse("joindomains: the TXT record was not found", i18n.ErrorsJoinDomainRecordMissing)
	// ErrHeldElsewhere means another Workspace has the domain verified and still publishes
	// its record.
	ErrHeldElsewhere = validation.ConflictField("domain", "joindomains: another workspace has verified this domain", i18n.ErrorsJoinDomainHeld)
	// ErrNotFound means no Join domain of that id is in the Workspace.
	ErrNotFound = validation.NotFound("joindomains: join domain not found", i18n.ErrorsJoinDomainNotFound)
)

// constraints are the unique constraints of the Join domains (ADR 0106): a domain is added once
// per Workspace, and verified by one Workspace at a time.
var constraints = db.Declare(
	db.Refusal("joindomain_workspace_id_domain", ErrAlreadyExists),
	db.Refusal("joindomain_domain", ErrHeldElsewhere),
)

// notFound is ErrNotFound for ent's not-found error of the domain's row, err otherwise.
func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Record is the DNS TXT record an Owner publishes to prove a domain.
type Record struct {
	Host  string
	Value string
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	client *ent.Client
	bus    *events.Bus
	lookup domainname.TXTLookup
	now    func() time.Time
}

// New builds the module. lookup is the injectable TXT resolver Sending domain
// verification uses; now is the clock the check times are stamped with. A Workspace that loses a
// verified domain publishes events.JoinDomainLost in the transaction of the loss, which tells its
// Owners (ADR 0105).
func New(client *ent.Client, bus *events.Bus, lookup domainname.TXTLookup, now func() time.Time) *Module {
	return &Module{client: client, bus: bus, lookup: lookup, now: now}
}

// RecordOf derives the TXT record to publish for a stored domain.
func RecordOf(d *ent.JoinDomain) Record {
	return Record{Host: recordLabel + d.Domain, Value: d.Token}
}

// Verified reports whether the domain is currently verified.
func Verified(d *ent.JoinDomain) bool { return d.VerifiedAt != nil }

// List returns the Workspace's Join domains, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped) ([]*ent.JoinDomain, error) {
	return s.JoinDomain().Query().Order(ent.Asc(joindomain.FieldID)).All(ctx)
}

// Add stores a pending claim of the domain with a fresh token. The domain is canonicalized
// (lower case, IDN as punycode); a malformed one is ErrInvalid, one the Workspace
// already has is ErrAlreadyExists. Another Workspace's claim, pending or verified, never
// blocks it.
func (m *Module) Add(ctx context.Context, s *ent.Scoped, raw string) (*ent.JoinDomain, error) {
	domain, ok := domainname.Normalize(raw)
	if !ok {
		return nil, ErrInvalid
	}
	token := "sphericon-join=" + rand.Text()

	var row *ent.JoinDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return constraints.Write(ctx, ts, func() error {
			var cerr error
			row, cerr = ts.JoinDomain().Create().SetDomain(domain).SetToken(token).Save(ctx)
			return cerr
		})
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Remove deletes the claim, and only the claim: Memberships are never touched. An unknown
// id is ErrNotFound.
func (m *Module) Remove(ctx context.Context, s *ent.Scoped, id int64) error {
	return notFound(s.JoinDomain().DeleteOneID(id).Exec(ctx))
}

// Verify checks the TXT record live and verifies the domain when it is published. A domain
// another Workspace has verified is re-checked at the holder first: still published is
// ErrHeldElsewhere, gone moves the domain to this Workspace in one transaction, the holder's row
// becoming pending (only from the verification found gone). An unpublished record is
// ErrRecordNotFound; a resolver failure is returned as is and changes nothing. A domain already
// verified here, or verified by a concurrent Verify, is returned as it is stored. An unknown id is
// ErrNotFound.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.JoinDomain, error) {
	row, err := s.JoinDomain().Get(ctx, id)
	if err != nil {
		return nil, notFound(err)
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
	// Another Workspace that verified the domain between the lookup and the write is
	// ErrHeldElsewhere (joindomain_domain).
	markVerified := func(to *ent.Scoped) error {
		return constraints.Write(ctx, to, func() error {
			return verification(to, row, now).FireCtx(ctx, prove)
		})
	}
	if holder == nil {
		err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error { return markVerified(ts) })
	} else {
		// The holder's row goes first: the verified-only unique index admits one at a time. Only
		// the verification whose record was just found gone is cleared.
		err = m.bus.WithinHandOverTx(ctx, s, holder.WorkspaceID, func(to, from *ent.Scoped, pub events.Publisher) error {
			err := verification(from, holder, now).FireCtx(ctx, lose)
			switch {
			case errors.Is(err, lifecycle.ErrInvalidTransition):
				// The holder's verification read is gone: a re-check or another hand-over cleared it,
				// and told its Owners. If the holder verified the domain again since, its new
				// verification stays and this Workspace's write is ErrHeldElsewhere (joindomain_domain).
			case err != nil:
				return err
			default:
				if err := pub.Publish(ctx, lost(holder)); err != nil {
					return err
				}
			}
			return markVerified(to)
		})
	}
	// A concurrent Verify that got there first leaves the stored row as the answer.
	if err != nil && !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil, err
	}
	verified, err := s.JoinDomain().Get(ctx, id)
	return verified, notFound(err)
}

// Due returns up to limit verified Join domains to re-check, across every Workspace, each with
// only its id and its Workspace (the check job names both), least-recently-checked first with
// never-checked ones before the rest (plain ascending order would put NULLs last in Postgres).
func (m *Module) Due(ctx context.Context, limit int) ([]*ent.JoinDomain, error) {
	return m.client.JoinDomain.Query().
		Where(joindomain.VerifiedAtNotNil()).
		Order(joindomain.ByLastCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst())).
		Limit(limit).
		Select(joindomain.FieldID, joindomain.FieldWorkspaceID).
		All(ctx)
}

// Recheck re-reads a verified domain's TXT record. A record that is gone (or carries
// another value) reverts the domain to unverified at once, under the system actor, and
// publishes events.JoinDomainLost in the same transaction; it reports whether it did. Only the
// verification it read is reverted: one made since (the domain handed over and verified again)
// stays. A resolver failure is returned and changes nothing, so the job retries. A domain that is
// not verified (it changed since the job was queued) is left alone. Memberships are never touched.
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
	system := m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
	err = m.bus.WithinScopedTx(ctx, system, func(ts *ent.Scoped, pub events.Publisher) error {
		if err := verification(ts, row, m.now()).FireCtx(ctx, lose); err != nil {
			return err
		}
		return pub.Publish(ctx, lost(row))
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		// Lost the swap: another re-check or a hand-over already reverted the verification read,
		// and told the Owners, or the domain was verified again since.
		return false, nil
	}
	if err != nil {
		return false, err
	}
	slog.WarnContext(ctx, "join domain verification lost",
		"join_domain_id", id, "workspace_id", row.WorkspaceID, "domain", row.Domain)
	return true, nil
}

// lost is the event of the Join domain row, as it was read, losing its verification.
func lost(row *ent.JoinDomain) *events.JoinDomainLost {
	return &events.JoinDomainLost{WorkspaceID: row.WorkspaceID, JoinDomainID: row.ID, Domain: row.Domain, Record: RecordOf(row).Host}
}

// Holder returns the Join domain row that has the domain verified, across every
// Workspace, or nil when none does. It is the Instance-wide lookup that runs before any
// Workspace scope exists.
func (m *Module) Holder(ctx context.Context, domain string) (*ent.JoinDomain, error) {
	return HolderIn(ctx, m.client, domain)
}

// HolderIn is Holder over a given client, so a transaction can read the holder it is about
// to join people to (the domain-join step of the users module). It reads the stored
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

// DevRecords is the Join domains' source of the end-to-end suite's resolver (devdns): a Join
// domain's record name, _sphericon-join.<domain>., answers the tokens every Workspace's claim of the
// domain stores, pending or verified, so a domain an Owner adds in the members settings verifies
// without DNS. Because it answers every Workspace's token, two Workspaces claiming one domain both
// verify, so a hand-over (the loser of a take-over) can never fire there. It reads across
// Workspaces, before any scope exists.
func DevRecords(client *ent.Client) func(ctx context.Context, name string) ([]string, error) {
	return func(ctx context.Context, name string) ([]string, error) {
		domain, ok := strings.CutPrefix(name, recordLabel)
		if !ok {
			return nil, nil
		}
		return client.JoinDomain.Query().
			Where(joindomain.Domain(strings.TrimSuffix(domain, "."))).
			Select(joindomain.FieldToken).
			Strings(ctx)
	}
}
