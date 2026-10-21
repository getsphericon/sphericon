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
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/liveuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/qmuntal/stateless"
	"github.com/samber/lo"
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
	// trackingTarget is the edge host the Tracking domain CNAME points at
	// (SPHERICON_TRACKING_CNAME_TARGET); empty turns the feature off (ADR 0038).
	trackingTarget string
}

func New(bus *events.Bus, cipher *secrets.Cipher, verify VerifyEnqueuer, trackingTarget string) *Module {
	return &Module{bus: bus, cipher: cipher, verify: verify, trackingTarget: trackingTarget}
}

// TrackingHost is the Tracking domain of a Sending domain: track.<domain> (ADR 0038).
func TrackingHost(domain string) string { return trackingLabel + domain }

const trackingLabel = "track."

// DomainOfTrackingHost is the inverse of TrackingHost: the Sending domain a Tracking
// domain host belongs to, lowercased. ok is false for anything that is not such a host.
func DomainOfTrackingHost(host string) (string, bool) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	domain, found := strings.CutPrefix(host, trackingLabel)
	if !found || domain == "" || strings.ContainsAny(domain, "/:@ ?#") {
		return "", false
	}
	return domain, true
}

// ServingTrackingHost returns the Tracking domain of the Workspace's Sending domain when
// its state is serving, and "" otherwise (including an unknown domain): mail builds its
// links on that host, or on the Instance's host when this is empty (ADR 0038).
func ServingTrackingHost(ctx context.Context, s *ent.Scoped, domain string) (string, error) {
	ok, err := s.SendingDomain().Query().
		Where(sendingdomain.Domain(domain), sendingdomain.TrackingStateEQ(sendingdomain.TrackingStateServing)).
		Exist(ctx)
	if err != nil || !ok {
		return "", err
	}
	return TrackingHost(domain), nil
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
	domain, ok := sending.NormalizeDomain(in.Domain)
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

// Archive discards a Sending domain (ADR 0049): it leaves the lists, serves no Tracking domain
// host, and keeps the record of mail sent from it. Live work that sends from it refuses it with an
// *inuse.Error naming that work. A domain is used through the domain of a From address, not an
// id: a scheduled or sending Broadcast uses the domain of its From address, and when it has none,
// of the default Integration's configured one; an active Automation that sends email uses the
// domain of that configured From, as its steps carry no From of their own. A sent or failed
// Broadcast is history and does not block. Archiving an archived domain changes nothing; an
// unknown id is ent's not-found error.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.SendingDomain().Get(ctx, id)
		if err != nil {
			return err
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

// Unarchive brings an archived Sending domain back and starts a new DNS check. The Tracking
// domain starts over from pending, so the host serves again only once the check proves it:
// the certificate and the serving state are not restored. It returns ErrAlreadyExists when the
// Workspace added the same domain again meanwhile. Restoring a domain that is not archived
// changes nothing; an unknown id is ent's not-found error.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	restored := false
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.SendingDomain().Get(ctx, id)
		if err != nil {
			return err
		}
		if row.DiscardedAt == nil {
			return nil
		}
		taken, err := ts.SendingDomain().Query().Where(sendingdomain.Domain(row.Domain)).Exist(ctx)
		if err != nil {
			return err
		}
		if taken {
			return ErrAlreadyExists
		}
		if row.TrackingState != sendingdomain.TrackingStatePending {
			err := TrackingMachine(ts, row, time.Now()).FireCtx(ctx, sendingdomain.TrackingStatePending)
			if err != nil {
				return err
			}
		}
		restored = true
		return ts.SendingDomain().Undiscard(ctx, id)
	})
	if db.IsUniqueViolation(err) {
		return ErrAlreadyExists
	}
	if err != nil || !restored {
		return err
	}
	return m.verify.EnqueueSendingDomainVerify(ctx, id)
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
	sending, err := liveuse.EmailAutomations(ctx, s)
	if err != nil {
		return nil, err
	}
	return append(users, sending...), nil
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

// TrackingRecord is the Tracking domain CNAME to publish: Host points at Target (the
// Instance's edge host). State and the two times are read-only facts the checks keep.
type TrackingRecord struct {
	Host           string
	Target         string
	State          sendingdomain.TrackingState
	Reason         *sendingdomain.TrackingReason
	CheckedAt      *time.Time
	StateChangedAt *time.Time
}

// DNSRecords are the records a Sending domain needs: DKIM gates sending, SPF and DMARC
// are advisory. Tracking is nil while the Tracking domain feature is off.
type DNSRecords struct {
	DKIM     Record
	SPF      Record
	DMARC    Record
	Tracking *TrackingRecord
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
	if m.trackingTarget != "" {
		rec.Tracking = &TrackingRecord{
			Host:           TrackingHost(d.Domain),
			Target:         m.trackingTarget,
			State:          d.TrackingState,
			Reason:         d.TrackingReason,
			CheckedAt:      d.TrackingCheckedAt,
			StateChangedAt: d.TrackingStateChangedAt,
		}
	}
	return rec
}

func validSelector(raw string) (string, error) {
	sel := strings.ToLower(strings.TrimSpace(raw))
	if !selectorPattern.MatchString(sel) {
		return "", &ValidationError{Field: "dkimSelector", Message: i18n.T("errors.dkim_selector_invalid", nil)}
	}
	return sel, nil
}

// List returns one page of the Workspace's Sending domains, ascending by id: the kept ones, or
// with archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.SendingDomain], error) {
	rows := func() *ent.SendingDomainQuery {
		if archived {
			return s.SendingDomain().QueryDiscarded()
		}
		return s.SendingDomain().Query()
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.SendingDomain, error) {
			return rows().Order(ent.Asc(sendingdomain.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// TrackingMachine is the Tracking domain's lifecycle (ADR 0046): every state can move to
// each of the others, so six edges, promotions and demotions alike. A trigger is the state it
// moves to. The machine reads the state the row had when the check loaded it, and the write
// is a compare-and-swap from that state: a check judged on a state another check already
// changed fails with lifecycle.ErrInvalidTransition and stores nothing.
func TrackingMachine(ts *ent.Scoped, row *ent.SendingDomain, now time.Time) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[sendingdomain.TrackingState]{
		Read: func(context.Context) (sendingdomain.TrackingState, error) { return row.TrackingState, nil },
		Write: func(ctx context.Context, from, to sendingdomain.TrackingState) (bool, error) {
			n, err := ts.SendingDomain().Update().
				Where(sendingdomain.ID(row.ID), sendingdomain.TrackingStateEQ(from)).
				SetTrackingCheckedAt(now).ClearTrackingReason().
				SetTrackingState(to).SetTrackingStateChangedAt(now).Save(ctx)
			return n == 1, err
		},
	})
	states := []sendingdomain.TrackingState{
		sendingdomain.TrackingStatePending, sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing,
	}
	for _, from := range states {
		cfg := m.Configure(from)
		for _, to := range states {
			if to != from {
				cfg.Permit(to, to)
			}
		}
	}
	return m
}
