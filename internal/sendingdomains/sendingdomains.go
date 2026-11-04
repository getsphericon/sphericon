// Package sendingdomains is the use-case module for a Workspace's Sending domains
// (ADR 0010): domain validation, DKIM keypair minting and sealing, the selector change
// rule, and triggering a live verification. /site and /api are thin adapters over it.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain refusal: validation.Error (a 422), ErrAlreadyExists (a
// 409), or ErrNotFound for an unknown id. The DKIM private key never leaves the module:
// callers see the public record only, through Records.
//
// Verification stays a live DKIM check done by a job; Verify only enqueues it. Nothing
// here, or in either API, can set a domain verified.
package sendingdomains

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/liveuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/validation"
)

// DefaultSelector is the DKIM selector used when the caller supplies none.
const DefaultSelector = "sphericon"

// The refusals of the module, one per meaning.
var (
	// ErrAlreadyExists means the Workspace already has this domain (Create).
	ErrAlreadyExists = validation.Conflict("sendingdomains: this domain is already added", i18n.ErrorsSendingDomainExists)
	// ErrNotFound means no Sending domain of that id is in the Workspace.
	ErrNotFound = validation.NotFound("sendingdomains: sending domain not found", i18n.ErrorsSendingDomainNotFound)
)

// notFound is ErrNotFound for ent's not-found error of the domain's row, err otherwise.
func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// selectorPattern is a DNS label sequence: letters, digits and hyphens, dot-separated.
var selectorPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// Verifier checks the DNS of one Sending domain at once, when a person asks for it.
type Verifier interface {
	VerifySendingDomain(ctx context.Context, sendingDomainID int64) error
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus    *events.Bus
	cipher *secrets.Cipher
	verify Verifier
}

func New(bus *events.Bus, cipher *secrets.Cipher, verify Verifier) *Module {
	return &Module{bus: bus, cipher: cipher, verify: verify}
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
	domain, ok := sending.NormalizeDomain(in.Domain)
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
	given, changes := in.Selector.Get()
	if changes {
		var err error
		if selector, err = validSelector(given); err != nil {
			return nil, err
		}
	}

	var row *ent.SendingDomain
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		cur, err := ts.SendingDomain().Get(ctx, id)
		if err != nil {
			return notFound(err)
		}
		row = cur
		if !changes || selector == cur.DkimSelector {
			return nil
		}
		if dkimstate.Is(cur) {
			// The published record is under the old selector: the verification is lost. The
			// loss lands only while the domain is still verified; a miss means a check moved
			// it meanwhile, and the plain edit below stands.
			row, err = ts.SendingDomain().UpdateOneID(id).Where(dkimstate.Verified()).
				SetDkimSelector(selector).SetVerificationLostAt(time.Now()).Save(ctx)
			if !ent.IsNotFound(err) {
				return err
			}
		}
		row, err = ts.SendingDomain().UpdateOneID(id).SetDkimSelector(selector).Save(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return row, nil
}

// Delete discards a Sending domain for good (ADR 0049): it leaves the lists and is never
// restored, and the record of mail sent from it stays. Live work that sends from it refuses the
// delete with an *inuse.Error naming that work. A domain is used through the domain of a From
// address, not an id: a scheduled or sending Broadcast uses the domain of its From address, and
// when it has none, of the default Integration's configured one; an active Automation that sends
// email uses the domain of that configured From, as its steps carry no From of their own. A sent or
// failed Broadcast is history and does not block. Deleting a deleted domain changes nothing; an
// unknown id is ErrNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.SendingDomain().Get(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if row.DiscardedAt != nil {
			return nil
		}
		users, err := m.liveUsers(ctx, ts, row.Domain)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return &inuse.Error{What: inuse.SubjectSendingDomain, Users: users}
		}
		return ts.SendingDomain().Discard(ctx, id)
	})
}

// liveUsers is the live work whose From address is on the domain.
func (m *Module) liveUsers(ctx context.Context, s *ent.Scoped, domain string) ([]inuse.User, error) {
	configured, err := m.configuredFromDomain(ctx, s)
	if err != nil {
		return nil, err
	}
	live, err := s.Broadcast().Query().
		Where(broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	onDomain := lo.Filter(live, func(b *ent.Broadcast, _ int) bool {
		from := messaging.DomainOf(lo.FromPtr(b.FromEmail))
		if b.FromEmail == nil || from == "" {
			from = configured
		}
		return from == domain
	})
	users := inuse.Broadcasts(onDomain)
	if configured != domain {
		return users, nil
	}
	automations, err := liveuse.SendingAutomations(ctx, s, messaging.ChannelEmail, liveuse.AnyIntegration)
	if err != nil {
		return nil, err
	}
	return append(users, automations...), nil
}

// configuredFromDomain is the domain of the From address the Workspace's default email
// Integration is configured with, "" when there is no such Integration or it has no From.
func (m *Module) configuredFromDomain(ctx context.Context, s *ent.Scoped) (string, error) {
	integ, err := messaging.DefaultEmailIntegration(ctx, s)
	if errors.Is(err, messaging.ErrNoProvider) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	raw, err := m.cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return "", err
	}
	var cfg struct {
		From string `json:"from"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	return messaging.DomainOf(cfg.From), nil
}

// Verify runs the live DNS check of the domain now and returns the domain as the check left
// it: `verified`, `lastCheckedAt`, `dmarcCheckedAt` and `dmarcFoundAt` are the outcome. It confirms the domain is in the
// Workspace first (ErrNotFound).
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	if _, err := s.SendingDomain().Get(ctx, id); err != nil {
		return nil, notFound(err)
	}
	if err := m.verify.VerifySendingDomain(ctx, id); err != nil {
		return nil, err
	}
	return s.SendingDomain().Get(ctx, id)
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
	dkimHost, dkimValue := sending.DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
	spfHost, spfValue := sending.SPFRecord(d.Domain)
	dmarcHost, dmarcValue := sending.DMARCRecord(d.Domain)
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
