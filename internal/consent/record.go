package consent

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/events"
)

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
}

// recordIn writes g as the Confirmation of its (Channel, Destination) unless one as strong or
// stronger stands, and publishes its Channel's proof (grant.proof) when it writes, in the
// transaction s and pub are bound to. A weaker or equal one leaves everything as it was and
// publishes nothing, so a repeated request is a no-op. It returns the Confirmation that stands.
func recordIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, g grant) (*ent.Confirmation, error) {
	standing, err := s.Confirmation().Query().Where(
		confirmation.ChannelEQ(g.channel),
		confirmation.DestinationEQ(g.destination),
	).Only(ctx)
	switch {
	case ent.IsNotFound(err):
	case err != nil:
		return nil, fmt.Errorf("find confirmation: %w", err)
	case !Stronger(g.provenance, standing.Provenance):
		return standing, nil
	default:
		// The stronger Confirmation replaces the row whole, so its evidence and time are its own;
		// the replaced one's Event stays in the log.
		if err := s.Confirmation().DeleteOneID(standing.ID).Exec(ctx); err != nil {
			return nil, fmt.Errorf("replace confirmation: %w", err)
		}
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
	created, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("record confirmation: %w", err)
	}
	if err := pub.Publish(ctx, proof); err != nil {
		return nil, err
	}
	return created, nil
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
