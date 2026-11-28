// Package sendingdomains is the domain module of a Workspace's Sending domains (ADR 0010):
// domain validation, DKIM keypair minting and sealing, the selector change rule, and the
// verified moments a check stores. /site and /api are thin adapters over it.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain refusal: ErrInvalidDomain (a 422), ErrAlreadyExists (a
// 409), or ErrNotFound for an unknown id. The DKIM private key never leaves the module:
// callers see the public record only, through Records.
//
// Verification stays a live DKIM check, with the DMARC lookup and the read of each email
// Integration's envelope next to it (ADR 0010, ADR 0012, ADR 0184): Verify and Recheck look the
// records up and store their outcome; nothing else stores one, so
// nothing in either API can set a domain verified. Deleting one is the use case
// internal/usecase/sendingdomaindeletion, which checks its Live use first (ADR 0102).
//
// It holds the raw *ent.Client only in Due, which lists the domains of every Workspace due a
// check before any scope exists, and in DevRecords, its source of the end-to-end suite's resolver
// (docs/architecture/workspace-scoping.md, Workspace scoping).
package sendingdomains

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
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
	ErrAlreadyExists = validation.ConflictField(fieldname.Domain, "sendingdomains: this domain is already added", i18n.ErrorsSendingDomainExists)
	// ErrNotFound means no Sending domain of that id is in the Workspace.
	ErrNotFound = validation.NotFound("sendingdomains: sending domain not found", i18n.ErrorsSendingDomainNotFound)
	// ErrInvalidDomain is a domain that is no domain name.
	ErrInvalidDomain = validation.RefuseField(fieldname.Domain, "sendingdomains: invalid domain", i18n.ErrorsSendingDomainInvalid)
	// ErrInvalidSelector is a DKIM selector that is no DNS label sequence.
	ErrInvalidSelector = validation.RefuseField(fieldname.DKIMSelector, "sendingdomains: invalid DKIM selector", i18n.ErrorsDkimSelectorInvalid)
)

// The unique constraint of the Sending domains (ADR 0106): a domain is added once per Workspace.
var constraints = db.Declare(
	db.Refusal("sendingdomain_workspace_id_domain", ErrAlreadyExists),
	db.Fault("sending_domain_envelopes_workspace_domain_integration", "a read of an envelope replaces the last one of its Sending domain and Integration (recordEnvelope)"),
)

// selectorPattern is a DNS label sequence: letters, digits and hyphens, dot-separated.
var selectorPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	// client reads the Sending domains of every Workspace, before any scope exists (Due).
	client *ent.Client
	bus    *events.Bus
	cipher *secrets.Cipher
	// dns resolves the DNS records a check reads.
	dns *DNS
	// integrations are the Workspace's email Integrations, whose envelopes a check reads.
	integrations *integrations.Module
	// now is the Instance clock a check and a lost verification are stamped by.
	now func() time.Time
}

// New builds the module over the raw client, which reads across Workspaces: bus opens the transactions its writes run in (a check's DKIM outcome is
// stored and its loss published in one, which tells the Owners, ADR 0105), lookup resolves the
// DNS records a check reads, integs are the Workspace's email Integrations whose envelopes
// a check reads (ADR 0184), and now is the Instance clock.
func New(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, lookup domainname.TXTLookup, integs *integrations.Module, now func() time.Time) *Module {
	return &Module{client: client, bus: bus, cipher: cipher, dns: NewDNS(lookup), integrations: integs, now: now}
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
// private key sealed. The View's envelopes are unread until the domain's first check.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (View, error) {
	domain, ok := domainname.Normalize(in.Domain)
	if !ok {
		return View{}, ErrInvalidDomain
	}
	selector := DefaultSelector
	if strings.TrimSpace(in.Selector) != "" {
		var err error
		if selector, err = validSelector(in.Selector); err != nil {
			return View{}, err
		}
	}

	privPEM, pubTXT, err := GenerateKeypair()
	if err != nil {
		return View{}, err
	}
	sealed, err := m.cipher.Encrypt(privPEM)
	if err != nil {
		return View{}, err
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
		return View{}, err
	}
	return m.viewOf(ctx, s, row)
}

// Update edits a Sending domain. Changing the selector moves the DKIM record, so the
// domain becomes unverified until the new record is published and checked. An unknown id, or a
// deleted domain's, is ErrNotFound, whatever the edit. The edit is a compare-and-swap on the domain
// as read: one that a check moved meanwhile is lifecycle.ErrInvalidTransition (a lost race, 409),
// never a selector change under a verification nobody checked.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (View, error) {
	var row *ent.SendingDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		cur, err := editable(ctx, ts, id)
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
		setSelector := func(u *ent.SendingDomainScopedUpdate) { u.SetDkimSelector(selector) }
		if dkimstate.Is(cur) {
			// The published record is under the old selector: the verification is lost
			// (DKIMSelectorMoved). The Owner made the edit, so no Owner is told.
			if _, err := moveDKIM(ctx, ts, cur, DKIMSelectorMoved, m.now(), setSelector); err != nil {
				return err
			}
		} else {
			upd := ts.SendingDomain().Update().Where(asJudged(cur)...)
			setSelector(upd)
			n, err := upd.Save(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				return lifecycle.ErrInvalidTransition
			}
		}
		row, err = ts.SendingDomain().Get(ctx, id)
		return err
	})
	if err != nil {
		return View{}, err
	}
	return m.viewOf(ctx, s, row)
}

// Record is one DNS TXT record to publish.
type Record struct {
	Host  string
	Value string
}

// DNSRecords are the records a Sending domain needs: DKIM gates sending, DMARC is advisory. SPF is
// no record of the domain's: it is checked on each email Integration's envelope (Envelopes, ADR 0184).
type DNSRecords struct {
	DKIM  Record
	DMARC Record
}

// Records derives the DNS records for a stored domain. The public key only.
func (m *Module) Records(d *ent.SendingDomain) DNSRecords {
	dkimHost, dkimValue := DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
	dmarcHost, dmarcValue := DMARCRecord(d.Domain)
	rec := DNSRecords{
		DKIM:  Record{Host: dkimHost, Value: dkimValue},
		DMARC: Record{Host: dmarcHost, Value: dmarcValue},
	}
	return rec
}

func validSelector(raw string) (string, error) {
	sel := strings.ToLower(strings.TrimSpace(raw))
	if !selectorPattern.MatchString(sel) {
		return "", ErrInvalidSelector
	}
	return sel, nil
}

// editable returns the Sending domain of the id for its owner to change or check. A deleted one is
// read, never changed (ADR 0100): it is ErrNotFound, as an unknown id is.
func editable(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	if err != nil {
		return nil, db.NotFoundAs(err, ErrNotFound)
	}
	if row.DiscardedAt != nil {
		return nil, ErrNotFound
	}
	return row, nil
}

// Get returns the Sending domain with the id, deleted or not, with its envelopes; an unknown id, or
// one of another Workspace, is ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (View, error) {
	row, err := m.row(ctx, s, id)
	if err != nil {
		return View{}, err
	}
	return m.viewOf(ctx, s, row)
}

// row is the Sending domain with the id, deleted or not; an unknown id, or one of another
// Workspace, is ErrNotFound.
func (m *Module) row(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	row, err := s.SendingDomain().Get(ctx, id)
	return row, db.NotFoundAs(err, ErrNotFound)
}

// List returns one page of the Workspace's kept Sending domains, ascending by id, each with its
// envelopes.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[View], error) {
	page, err := pagination.List(ctx, p, s.SendingDomain().Query().Order(ent.Asc(sendingdomain.FieldID)))
	if err != nil {
		return pagination.Page[View]{}, err
	}
	envelopes, err := m.envelopes(ctx, s, page.Items)
	if err != nil {
		return pagination.Page[View]{}, err
	}
	return page.Map(func(d *ent.SendingDomain) View {
		return View{Domain: d, Envelopes: envelopes[d.ID]}
	}), nil
}
