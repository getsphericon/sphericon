// Package sendingdomains is the use-case module for a Workspace's Sending domains
// (ADR 0010): domain validation, DKIM keypair minting and sealing, the selector change
// rule, and triggering a live verification. /site and /api are thin adapters over it.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain error: ValidationError (a 422), ErrAlreadyExists (a 409), or
// ent's not-found for an unknown id. The DKIM private key never leaves the module:
// callers see the public record only, through Records.
//
// Verification stays a live DKIM check done by a job; Verify only enqueues it. Nothing
// here, or in either API, can set a domain verified.
package sendingdomains

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"

	"golang.org/x/net/idna"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
)

// DefaultSelector is the DKIM selector used when the caller supplies none.
const DefaultSelector = "sphericon"

// ErrAlreadyExists means the Workspace already has this domain.
var ErrAlreadyExists = errors.New("this domain is already added")

// selectorPattern is a DNS label sequence: letters, digits and hyphens, dot-separated.
var selectorPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// ValidationError is a refused input: Field names the request field, Message is what to
// show for it.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// VerifyEnqueuer schedules an immediate DKIM re-check of one Sending domain.
type VerifyEnqueuer interface {
	EnqueueSendingDomainVerify(ctx context.Context, sendingDomainID int64) error
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus    *events.Bus
	cipher *secrets.Cipher
	verify VerifyEnqueuer
}

func New(bus *events.Bus, cipher *secrets.Cipher, verify VerifyEnqueuer) *Module {
	return &Module{bus: bus, cipher: cipher, verify: verify}
}

// CreateInput is a new Sending domain. An empty Selector means DefaultSelector.
type CreateInput struct {
	Domain   string
	Selector string
}

// UpdateInput is a partial edit; nil fields keep the stored value. The domain name is
// immutable: the key is minted for it.
type UpdateInput struct {
	Selector *string
}

// Create validates the domain, mints its DKIM keypair and stores the row with the
// private key sealed.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.SendingDomain, error) {
	domain, ok := normalize(in.Domain)
	if !ok {
		return nil, &ValidationError{Field: "domain", Message: i18n.T("errors.sending_domain_invalid", nil)}
	}
	selector := DefaultSelector
	if strings.TrimSpace(in.Selector) != "" {
		var err error
		if selector, err = validSelector(in.Selector); err != nil {
			return nil, err
		}
	}

	privPEM, pubTXT, err := sending.GenerateKeypair()
	if err != nil {
		return nil, err
	}
	sealed, err := m.cipher.Encrypt(privPEM)
	if err != nil {
		return nil, err
	}

	// Inside a scoped transaction (a savepoint) so a unique violation rolls back only
	// this write, and so the change is captured at the scoped client (ADR 0022).
	var row *ent.SendingDomain
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var cerr error
		row, cerr = ts.SendingDomain().Create().
			SetDomain(domain).
			SetDkimSelector(selector).
			SetDkimPrivateKeyEncrypted(sealed).
			SetDkimPublicKey(pubTXT).
			Save(ctx)
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

// Update edits a Sending domain. Changing the selector moves the DKIM record, so the
// domain becomes unverified until the new record is published and checked.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.SendingDomain, error) {
	var selector string
	if in.Selector != nil {
		var err error
		if selector, err = validSelector(*in.Selector); err != nil {
			return nil, err
		}
	}

	var row *ent.SendingDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		cur, err := ts.SendingDomain().Get(ctx, id)
		if err != nil {
			return err
		}
		row = cur
		if in.Selector == nil || selector == cur.DkimSelector {
			return nil
		}
		row, err = ts.SendingDomain().UpdateOneID(id).
			SetDkimSelector(selector).
			SetVerified(false).
			Save(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Delete removes a Sending domain.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return ts.SendingDomain().DeleteOneID(id).Exec(ctx)
	})
}

// Verify schedules a live DKIM check of the domain. It confirms the domain is in the
// Workspace first, and reports nothing about the outcome: the check sets `verified`.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) error {
	if _, err := s.SendingDomain().Get(ctx, id); err != nil {
		return err
	}
	return m.verify.EnqueueSendingDomainVerify(ctx, id)
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
func Records(d *ent.SendingDomain) DNSRecords {
	dkimHost, dkimValue := sending.DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
	spfHost, spfValue := sending.SPFRecord(d.Domain)
	dmarcHost, dmarcValue := sending.DMARCRecord(d.Domain)
	return DNSRecords{
		DKIM:  Record{Host: dkimHost, Value: dkimValue},
		SPF:   Record{Host: spfHost, Value: spfValue},
		DMARC: Record{Host: dmarcHost, Value: dmarcValue},
	}
}

func validSelector(raw string) (string, error) {
	sel := strings.ToLower(strings.TrimSpace(raw))
	if !selectorPattern.MatchString(sel) {
		return "", &ValidationError{Field: "dkimSelector", Message: i18n.T("errors.dkim_selector_invalid", nil)}
	}
	return sel, nil
}

// normalize validates raw as a registrable domain name (IDNA registration rules: label
// syntax, length limits, punycode) with at least one dot and not an IP address, and
// returns its ASCII (punycode) form, the form DNS and DKIM use.
func normalize(raw string) (string, bool) {
	d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
	if !strings.Contains(d, ".") || net.ParseIP(d) != nil {
		return "", false
	}
	ascii, err := idna.Registration.ToASCII(d)
	if err != nil {
		return "", false
	}
	return ascii, true
}

// List returns one page of the Workspace's Sending domains, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.SendingDomain], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.SendingDomain().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.SendingDomain, error) {
			return s.SendingDomain().Query().Order(ent.Asc(sendingdomain.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
