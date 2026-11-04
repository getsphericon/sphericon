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
	"regexp"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
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

// Senders builds the EmailSender of an Integration, as a send through it does: the Live use check
// reads the From it is configured with from it. *messaging.Resolver satisfies it.
type Senders interface {
	EmailSender(ctx context.Context, s *ent.Scoped, integ *ent.Integration) (messaging.EmailSender, error)
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus     *events.Bus
	cipher  *secrets.Cipher
	verify  Verifier
	senders Senders
}

func New(bus *events.Bus, cipher *secrets.Cipher, verify Verifier, senders Senders) *Module {
	return &Module{bus: bus, cipher: cipher, verify: verify, senders: senders}
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
// domain becomes unverified until the new record is published and checked. An unknown id, or a
// deleted domain's, is ErrNotFound, whatever the edit.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.SendingDomain, error) {
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
		if dkimstate.Is(cur) {
			// The published record is under the old selector: the verification is lost. The
			// loss lands only while the domain is still verified; a miss means a check moved
			// it meanwhile, and the plain edit below stands.
			row, err = ts.SendingDomain().UpdateOneID(id).Where(dkimstate.Verified(), sendingdomain.DiscardedAtIsNil()).
				SetDkimSelector(selector).SetVerificationLostAt(time.Now()).Save(ctx)
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

// Delete discards a Sending domain for good (ADR 0049): it leaves the lists and is never
// restored, and the record of mail sent from it stays. Its Live use (ADR 0099) refuses the delete
// with an *inuse.Error naming that work. A domain is used through the domain of a From address, not
// an id: a scheduled or sending Broadcast, the live version of an active Automation or a version an
// unfinished Enrollment stands on uses the domain of the From its email goes out with, its own or,
// without one, the From of the Integration it goes through (an Automation step carries none). A sent
// or failed Broadcast is history and does not block. Deleting a deleted domain changes nothing; an
// unknown id is ErrNotFound. The check and the discard hold the Workspace's lock (ADR 0099), so no
// work becomes live between them.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
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

// liveUsers is the Live use of the domain (ADR 0099): the work whose email goes out from an address
// on it, its own From or, without one, the From of the Integration it goes through, by the same
// resolution a send takes (messaging.Through, messaging.EffectiveFrom).
func (m *Module) liveUsers(ctx context.Context, s *ent.Scoped, domain string) ([]inuse.User, error) {
	sends, err := liveuse.Sends(ctx, s)
	if err != nil {
		return nil, err
	}
	senders := map[int64]messaging.EmailSender{}
	var onDomain []liveuse.Send
	for _, send := range sends {
		if send.Channel != messaging.ChannelEmail {
			continue
		}
		var sender messaging.EmailSender
		if send.Integration != nil {
			if sender = senders[send.Integration.ID]; sender == nil {
				if sender, err = m.senders.EmailSender(ctx, s, send.Integration); err != nil {
					return nil, err
				}
				senders[send.Integration.ID] = sender
			}
		}
		if from, _ := messaging.EffectiveFrom(sender, send.From, ""); messaging.DomainOf(from) == domain {
			onDomain = append(onDomain, send)
		}
	}
	return liveuse.Users(onDomain), nil
}

// Verify runs the live DNS check of the domain now and returns the domain as the check left
// it: `verified`, `lastCheckedAt`, `dmarcCheckedAt` and `dmarcFoundAt` are the outcome. It confirms the domain is in the
// Workspace and kept first: an unknown or a deleted domain is ErrNotFound.
func (m *Module) Verify(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
	if _, err := editable(ctx, s, id); err != nil {
		return nil, err
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

// editable returns the Sending domain of the id for its owner to change. A deleted one is read,
// never changed (ADR 0100): it is ErrNotFound, as an unknown id is.
func editable(ctx context.Context, s *ent.Scoped, id int64) (*ent.SendingDomain, error) {
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
