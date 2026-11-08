// Package sendingdomains is the domain module of a Workspace's Sending domains (ADR 0010):
// domain validation, DKIM keypair minting and sealing, the selector change rule, and the
// verified moments a check stores. /site and /api are thin adapters over it.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain refusal: validation.Error (a 422), ErrAlreadyExists (a
// 409), or ErrNotFound for an unknown id. The DKIM private key never leaves the module:
// callers see the public record only, through Records.
//
// Verification stays a live DKIM check, with the DMARC lookup next to it (ADR 0010, ADR 0012):
// Verify and Recheck look the records up and store their outcome; nothing else stores one, so
// nothing in either API can set a domain verified. Deleting one is the use case
// internal/usecase/sendingdomaindeletion, which checks its Live use first (ADR 0102).
//
// It holds the raw *ent.Client only in Due, which lists the domains of every Workspace due a
// check before any scope exists, and in DevTXTLookup, the end-to-end suite's resolver
// (docs/architecture.md, Workspace scoping).
package sendingdomains

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// DefaultSelector is the DKIM selector used when the caller supplies none.
const DefaultSelector = "sphericon"

// The refusals of the module, one per meaning.
var (
	// ErrAlreadyExists means the Workspace already has this domain (Create).
	ErrAlreadyExists = validation.ConflictField("domain", "sendingdomains: this domain is already added", i18n.ErrorsSendingDomainExists)
	// ErrNotFound means no Sending domain of that id is in the Workspace.
	ErrNotFound = validation.NotFound("sendingdomains: sending domain not found", i18n.ErrorsSendingDomainNotFound)
)

// The unique constraint of the Sending domains (ADR 0106): a domain is added once per Workspace.
var constraints = db.Declare(db.Refusal("sendingdomain_workspace_id_domain", ErrAlreadyExists))

// notFound is ErrNotFound for ent's not-found error of the domain's row, err otherwise.
func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// selectorPattern is a DNS label sequence: letters, digits and hyphens, dot-separated.
var selectorPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus    *events.Bus
	cipher *secrets.Cipher
	// lookup resolves the DNS records a check reads.
	lookup domainname.TXTLookup
	// now is the Instance clock a check and a lost verification are stamped by.
	now func() time.Time
}

// New builds the module: bus opens the transactions its writes run in (a check's DKIM outcome is
// stored and its loss published in one, which tells the Owners, ADR 0105), lookup resolves the
// DNS records a check reads, and now is the Instance clock.
func New(bus *events.Bus, cipher *secrets.Cipher, lookup domainname.TXTLookup, now func() time.Time) *Module {
	return &Module{bus: bus, cipher: cipher, lookup: lookup, now: now}
}

// CreateInput is a new Sending domain. An empty Selector means DefaultSelector.
type CreateInput struct {
	Domain   string
	Selector string
}

// UpdateInput is a patch of a Sending domain: a field that keeps leaves the stored value. The
// domain name is immutable: the key is minted for it. The selector cannot be cleared.
type UpdateInput struct {
	Selector patch.Field[string]
}

// Create validates the domain, mints its DKIM keypair and stores the row with the
// private key sealed.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.SendingDomain, error) {
	domain, ok := domainname.Normalize(in.Domain)
	if !ok {
		return nil, &validation.Error{Field: "domain", Message: i18n.T(i18n.ErrorsSendingDomainInvalid, nil)}
	}
	selector := DefaultSelector
	if strings.TrimSpace(in.Selector) != "" {
		var err error
		if selector, err = validSelector(in.Selector); err != nil {
			return nil, err
		}
	}

	privPEM, pubTXT, err := GenerateKeypair()
	if err != nil {
		return nil, err
	}
	sealed, err := m.cipher.Encrypt(privPEM)
	if err != nil {
		return nil, err
	}

	// Inside a scoped transaction, so the change is captured at the scoped client (ADR 0022).
	var row *ent.SendingDomain
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return constraints.Write(ctx, ts, func() (err error) {
			row, err = ts.SendingDomain().Create().
				SetDomain(domain).
				SetDkimSelector(selector).
				SetDkimPrivateKeyEncrypted(sealed).
				SetDkimPublicKey(pubTXT).
				Save(ctx)
			return err
		})
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Update edits a Sending domain. Changing the selector moves the DKIM record, so the
// domain becomes unverified until the new record is published and checked. An unknown id, or a
// deleted domain's, is ErrNotFound, whatever the edit.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.SendingDomain, error) {
	var row *ent.SendingDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		cur, err := Editable(ctx, ts, id)
		if err != nil {
			return err
		}
		row = cur
		given, changes := in.Selector.Get()
		if !changes {
			return nil
		}
		selector, err := validSelector(given)
		if err != nil {
			return err
		}
		if selector == cur.DkimSelector {
			return nil
		}
		if dkimstate.Is(cur) {
			// The published record is under the old selector: the verification is lost. The
			// loss lands only while the domain is still verified; a miss means a check moved
			// it meanwhile, and the plain edit below stands.
			row, err = ts.SendingDomain().UpdateOneID(id).Where(dkimstate.Verified(), sendingdomain.DiscardedAtIsNil()).
				SetDkimSelector(selector).SetVerificationLostAt(m.now()).Save(ctx)
			if !ent.IsNotFound(err) {
				return err
			}
		}
		// A Delete that landed meanwhile leaves no row to edit.
		row, err = ts.SendingDomain().UpdateOneID(id).Where(sendingdomain.DiscardedAtIsNil()).SetDkimSelector(selector).Save(ctx)
		return notFound(err)
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// DeleteIn discards the Sending domain row for good (ADR 0049) on the transaction's scoped client,
// without asking whether work still sends from it: the unchecked removal behind the Sending domain
// deletion use case (internal/usecase/sendingdomaindeletion), which runs the Removal check (its
// Live use) under the Workspace lock first and is the only caller easement allows. The record of mail sent from it
// stays.
func DeleteIn(ctx context.Context, ts *ent.Scoped, id int64, at time.Time) error {
	return ts.SendingDomain().Discard(ctx, id, at)
}

// Record is one DNS TXT record to publish.
type Record struct {
	Host  string
	Value string
}

// DNSRecords are the records a Sending domain needs: DKIM gates sending, SPF and DMARC
// are advisory.
type DNSRecords struct {
	DKIM  Record
	SPF   Record
	DMARC Record
}

// Records derives the DNS records for a stored domain. The public key only.
func (m *Module) Records(d *ent.SendingDomain) DNSRecords {
	dkimHost, dkimValue := DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
	spfHost, spfValue := SPFRecord(d.Domain)
	dmarcHost, dmarcValue := DMARCRecord(d.Domain)
	rec := DNSRecords{
		DKIM:  Record{Host: dkimHost, Value: dkimValue},
		SPF:   Record{Host: spfHost, Value: spfValue},
		DMARC: Record{Host: dmarcHost, Value: dmarcValue},
	}
	return rec
}

func validSelector(raw string) (string, error) {
	sel := strings.ToLower(strings.TrimSpace(raw))
	if !selectorPattern.MatchString(sel) {
		return "", &validation.Error{Field: "dkimSelector", Message: i18n.T(i18n.ErrorsDkimSelectorInvalid, nil)}
	}
	return sel, nil
}

// Editable returns the Sending domain of the id for its owner to change or check. A deleted one is
// read, never changed (ADR 0100): it is ErrNotFound, as an unknown id is.
func Editable(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if row.DiscardedAt != nil {
		return nil, ErrNotFound
	}
	return row, nil
}

// Get returns the Sending domain with the id, deleted or not; an unknown id, or one of another
// Workspace, is ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	return row, notFound(err)
}

// List returns one page of the Workspace's kept Sending domains, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.SendingDomain], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.SendingDomain().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.SendingDomain, error) {
			return s.SendingDomain().Query().Order(ent.Asc(sendingdomain.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
