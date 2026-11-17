// Package confirmations is the one place a Confirmation (confirmed opt-in) is written (ADR 0013,
// ADR 0077, ADR 0102): the derived destination-keyed row with the immutable confirmed Event of its
// Channel. It records the consent a double opt-in link, a START texted to the Workspace or the
// evidence an API client collected gives, reads the SMS consent of a Contact's phone, and drops a
// Confirmation an opt-out of everything ends. On SMS the opt-in and the opt-out are one number's:
// a START lifts the opt-out (unsubscribes) as it records the Confirmation.
package confirmations

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module records, reads and withdraws the Confirmations of a Workspace. It publishes each
// Confirmation's proof in the transaction of the scope it is given, through the bus.
type Module struct {
	bus *events.Bus
	// unsubs is the opt-outs a START lifts and the SMS consent of a phone reads.
	unsubs *unsubscribes.Module
	now    func() time.Time
}

// New builds the confirmations module over the bus whose transactions its operations join, the
// opt-outs and the Instance clock, which dates a consent given now.
func New(bus *events.Bus, unsubs *unsubscribes.Module, now func() time.Time) *Module {
	return &Module{bus: bus, unsubs: unsubs, now: now}
}

// strength ranks how a Confirmation was obtained, the strongest highest (ADR 0077): the person
// clicking the double opt-in link, the person texting START, consent an API client or an import
// collected with its evidence, and last a prior basis an admin asserted when confirmed opt-in was
// turned on.
var strength = map[confirmation.Provenance]int{
	confirmation.ProvenanceGrandfathered: 1,
	confirmation.ProvenanceImported:      2,
	confirmation.ProvenanceKeyword:       3,
	confirmation.ProvenanceDoubleOptIn:   4,
}

// Stronger reports whether a Confirmation obtained as a is stronger evidence than one obtained as
// b: only a stronger one replaces a Confirmation that stands, so the evidence on record is the best
// the Workspace has.
func Stronger(a, b confirmation.Provenance) bool {
	return strength[a] > strength[b]
}

// Evidence is the evidence of consent collected outside sphericon (ADR 0077): the wording the
// person was shown, where it was collected and the brand they agreed to hear from. It is kept in
// the immutable sms.confirmed Event and projected on the Confirmation.
type Evidence struct {
	Wording, Source, Brand string
}

// grant is one Confirmation to record.
type grant struct {
	channel     channels.Channel
	destination string
	provenance  confirmation.Provenance
	// evidence is nil for a Confirmation that carries none (a double opt-in, a keyword).
	evidence *Evidence
	// contactID is the Contact the Destination belongs to, 0 when none does.
	contactID int64
	// ip is the confirming client's address, on a double opt-in.
	ip string
	// at is when the consent was given (ADR 0154).
	at consentstamps.Occurrence
}

// ErrSuperseded refuses a Confirmation given before the destination's consent was withdrawn: it
// changes nothing (ADR 0154).
var ErrSuperseded = validation.Conflict("consent: withdrawn after this confirmation was given", i18n.ErrorsConsentSuperseded)

// stampKey is the consent state a Confirmation of destination on channel sets.
func stampKey(channel channels.Channel, destination string) consentstamps.Key {
	return consentstamps.Key{Channel: channel, Destination: destination, Source: eligibility.SourceEverything}
}

// The unique constraint of the Confirmation's table (ADR 0106): one Confirmation
// per (Channel, Destination), which recordIn writes ON CONFLICT DO NOTHING.
var _ = db.Declare(
	db.Fault("confirmations_ws_channel_dest", "recordIn inserts ON CONFLICT (workspace_id, channel, destination) DO NOTHING"),
)

// recordIn writes g as the Confirmation of its (Channel, Destination) unless one as strong or
// stronger stands, and publishes its Channel's proof (grant.proof) when it writes, in the
// transaction s is bound to. A weaker or equal one leaves everything as it was and
// publishes nothing, so a repeated request is a no-op; so does one that a concurrent request beat
// to the row. It returns the Confirmation that stands. One given before the destination's
// Confirmation state was last set (an opt-out of everything withdrew it later) records nothing
// either, and is ErrSuperseded when no Confirmation stands (ADR 0154): provenance is checked first,
// then time.
func (m *Module) recordIn(ctx context.Context, s *ent.Scoped, g grant) (*ent.Confirmation, error) {
	key := stampKey(g.channel, g.destination)
	if _, err := consentstamps.Hold(ctx, s, key); err != nil {
		return nil, err
	}
	standing, err := s.Confirmation().Query().Where(
		confirmation.ChannelEQ(g.channel),
		confirmation.DestinationEQ(g.destination),
	).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		standing = nil
	case err != nil:
		return nil, fmt.Errorf("find confirmation: %w", err)
	case !Stronger(g.provenance, standing.Provenance):
		return standing, nil
	}
	admitted, err := consentstamps.Admit(ctx, s, key, consentstamps.Confirmation, g.at, false)
	switch {
	case err != nil:
		return nil, err
	case !admitted && standing == nil:
		return nil, ErrSuperseded
	case !admitted:
		return standing, nil
	case standing != nil:
		// The stronger Confirmation replaces the row whole, so its evidence and time are its own;
		// the replaced one's Event stays in the log.
		if err := s.Confirmation().DeleteOneID(standing.ID).Exec(ctx); err != nil {
			return nil, fmt.Errorf("replace confirmation: %w", err)
		}
	}
	// The Contact may be gone by now (erased between the send and the click): the Confirmation is
	// keyed by its destination and is recorded unlinked.
	if g.contactID, err = contactlock.Referenced(ctx, s, g.contactID); err != nil {
		return nil, fmt.Errorf("lock the confirmation's contact: %w", err)
	}
	create := s.Confirmation().Create().
		SetChannel(g.channel).
		SetDestination(g.destination).
		SetProvenance(g.provenance)
	if g.contactID != 0 {
		create.SetContactID(g.contactID)
	}
	if ev := g.evidence; ev != nil {
		create.SetWording(ev.Wording).SetSource(ev.Source).SetBrand(ev.Brand)
	}
	proof, err := g.proof(s.WorkspaceID())
	if err != nil {
		return nil, err
	}
	id, err := create.
		OnConflictColumns(confirmation.FieldWorkspaceID, confirmation.FieldChannel, confirmation.FieldDestination).
		DoNothing().
		ID(ctx)
	inserted, err := db.Inserted(err)
	if err != nil {
		return nil, fmt.Errorf("record confirmation: %w", err)
	}
	if !inserted {
		// A concurrent request recorded the destination first: its Confirmation stands, and its
		// proof is its own.
		return standingOf(ctx, s, g.channel, g.destination)
	}
	if err := m.bus.PublishIn(ctx, s, proof); err != nil {
		return nil, err
	}
	return s.Confirmation().Get(ctx, id)
}

// standingOf reads the Confirmation of destination on channel.
func standingOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (*ent.Confirmation, error) {
	standing, err := s.Confirmation().Query().Where(
		confirmation.ChannelEQ(channel),
		confirmation.DestinationEQ(destination),
	).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("read confirmation: %w", err)
	}
	return standing, nil
}

// proof is the immutable Event of g, its Channel's own: marketing.confirmed for an email address,
// sms.confirmed for a phone number. An Automation trigger is an action with no property filter, so
// one Channel's consent never starts what another's was meant to (a welcome email).
func (g grant) proof(workspaceID int64) (events.DomainEvent, error) {
	switch g.channel {
	case channels.Email:
		return &events.MarketingConfirmed{
			WorkspaceID: workspaceID, ContactID: g.contactID, Email: g.destination,
			Provenance: string(g.provenance), IP: g.ip,
		}, nil
	case channels.SMS:
		e := &events.SmsConfirmed{
			WorkspaceID: workspaceID, ContactID: g.contactID, Phone: g.destination, Provenance: string(g.provenance),
		}
		if ev := g.evidence; ev != nil {
			e.Wording, e.Source, e.Brand = ev.Wording, ev.Source, ev.Brand
		}
		return e, nil
	default:
		return nil, fmt.Errorf("record confirmation: no consent Event on channel %q", g.channel)
	}
}

// WithdrawInput is a withdrawal of the consent of a Destination on a Channel.
type WithdrawInput struct {
	Channel     channels.Channel
	Destination string
	// At is when the person withdrew (ADR 0154).
	At consentstamps.Occurrence
}

// Withdraw deletes the Confirmation of the Destination on the Channel: an opt-out of everything
// invalidates it (ADR 0013, ADR 0077), so stale consent never silently reactivates and only a new
// Confirmation makes the destination reachable again. The immutable confirmed Event stays as proof
// ("confirmed at T1, left at T2"). A withdrawal older than the Confirmation standing withdraws
// nothing (ADR 0154). Run it inside the transaction s is bound to.
func (m *Module) Withdraw(ctx context.Context, s *ent.Scoped, in WithdrawInput) error {
	admitted, err := consentstamps.Admit(ctx, s, stampKey(in.Channel, in.Destination), consentstamps.Confirmation, in.At, true)
	if err != nil || !admitted {
		return err
	}
	_, err = s.Confirmation().Delete().Where(
		confirmation.ChannelEQ(in.Channel),
		confirmation.DestinationEQ(in.Destination),
	).Exec(ctx)
	return err
}
