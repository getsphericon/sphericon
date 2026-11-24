// Package engagementtracking is the one place Engagement tracking consent is written (ADR 0120,
// ADR 0102): whether the opens and clicks of a Destination's messages may be tracked, `granted` or
// `denied`, per (Workspace, Channel, Destination). Its truth is the evidentiary
// `<channel>.engagement_tracking_granted` / `_denied` Event, published in the transaction of the
// derived row the send path and the recorder read. Observations are ordered as every consent
// fact's (ADR 0154), on the Destination's `everything` consent stamp: provenance first (a `denied`
// applies from anyone, and only the person's own grant lifts one, ADR 0121), then time (an older
// observation changes nothing and joins the Contact's history as contact.consent_superseded; a tie
// goes to the refusal). It also answers the effective consent of a Destination: its consent, else
// the Workspace's default.
package engagementtracking

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// State is whether a Destination agreed to be tracked.
type State = engagementtrackingconsent.State

// The states of a consent.
const (
	Granted = engagementtrackingconsent.StateGranted
	Denied  = engagementtrackingconsent.StateDenied
)

// Provenance is who recorded a consent: the person on a page reached from a message (page), a
// member on the Contact card (manual), the customer through the API or an import (imported), an
// On-site form's checkbox (onsite_form).
type Provenance = engagementtrackingconsent.Provenance

// The provenances of a consent.
const (
	Page       = engagementtrackingconsent.ProvenancePage
	Manual     = engagementtrackingconsent.ProvenanceManual
	Imported   = engagementtrackingconsent.ProvenanceImported
	OnsiteForm = engagementtrackingconsent.ProvenanceOnsiteForm
)

// Decision is whether a Destination's opens and clicks are tracked.
type Decision string

// The decisions: track, or don't.
const (
	Track     Decision = "track"
	DontTrack Decision = "dont_track"
)

// Reading is what a Destination's consent reads as: granted or denied as recorded, or default
// where nothing is recorded.
type Reading string

// The readings of a consent.
const (
	ReadingGranted Reading = "granted"
	ReadingDenied  Reading = "denied"
	ReadingDefault Reading = "default"
)

// Outcome is what became of an observation.
type Outcome string

// The outcomes of an observation: it set the consent, or it occurred before the consent standing
// and changed nothing (ADR 0154).
const (
	Applied    Outcome = "applied"
	Superseded Outcome = "superseded"
)

// The refusals of a consent observation.
var (
	// ErrRefusalStands refuses a grant over a standing `denied` that is not the person's own:
	// only the person lifts a refusal (ADR 0121).
	ErrRefusalStands = validation.Conflict("engagement tracking: the destination refused tracking, and only the person can lift a refusal",
		i18n.ErrorsEngagementTrackingRefused)
	// ErrChannel refuses a Channel whose Destinations take no consent yet.
	ErrChannel = validation.RefuseField(fieldname.Channel, "engagement tracking: only email takes a consent yet",
		i18n.ErrorsEngagementTrackingChannel)
	// ErrInFuture refuses a grant said to occur after now: it would outrank every real refusal.
	ErrInFuture = validation.RefuseField(fieldname.OccurredAt, "engagement tracking: occurredAt of a grant must not be in the future",
		i18n.ErrorsEngagementTrackingInFuture)
	// ErrNoEmail refuses a Contact's email consent for a Contact without an email address.
	ErrNoEmail = validation.RefuseField(fieldname.Email, "engagement tracking: the contact has no email", i18n.ErrorsContactEmailMissing)
	// ErrNotTheContacts refuses a member's observation, on the Contact card, of a Destination the
	// Contact does not hold.
	ErrNotTheContacts = validation.RefuseField(fieldname.Destination, "engagement tracking: the destination is not the contact's",
		i18n.ErrorsEngagementTrackingNotTheContacts)
	// ErrContactNotFound: no such Contact in the Workspace.
	ErrContactNotFound = validation.NotFound("engagement tracking: contact not found", i18n.ErrorsContactNotFound)
	// errState and errProvenance refuse a value outside their closed sets.
	errState      = validation.RefuseField(fieldname.State, "engagement tracking: unknown state", i18n.ErrorsEngagementTrackingState)
	errProvenance = validation.Refuse("engagement tracking: unknown provenance", i18n.ErrorsEngagementTrackingProvenance)
)

// The unique constraint of the consent's table (ADR 0106): one consent per (Channel, Destination),
// which a write reaches only under the Destination's consent stamp, so no two writers race it.
var _ = db.Declare(
	db.Fault("engagement_tracking_consents_ws_channel_dest", "every write holds the Destination's consent stamp first and reads the row under it"),
)

// open is the Channels whose Destinations take a consent today. SMS and Telegram open when their
// Destinations do (the row, the Events and the provenance already cover them).
var open = map[channels.Channel]bool{channels.Email: true}

// Module records and reads the Engagement tracking consent of a Workspace's Destinations. It
// publishes each consent's proof in the transaction of the scope it is given, through the bus.
type Module struct {
	bus    *events.Bus
	stamps *consentstamps.Module
	locks  *contactlock.Module
	now    func() time.Time
}

// New builds the module over the bus whose transactions its operations join and the Instance
// clock, which dates an observation that states no time.
func New(bus *events.Bus, now func() time.Time) *Module {
	return &Module{bus: bus, stamps: consentstamps.New(), locks: contactlock.New(), now: now}
}

// RecordInput is one observation of a Destination's consent.
type RecordInput struct {
	Channel     channels.Channel
	Destination string
	State       State
	Provenance  Provenance
	// OccurredAt is when the person gave or refused the consent; the zero time is now.
	OccurredAt time.Time
}

// Consent is the Engagement tracking consent of one Destination: what it reads as, who recorded
// it, and whether the Destination is tracked.
type Consent struct {
	Channel     channels.Channel
	Destination string
	State       Reading
	// Provenance and UpdatedAt are nil while nothing is recorded.
	Provenance *Provenance
	Effective  Decision
	UpdatedAt  *time.Time
}

// Recorded is what an observation did and the consent that stands after it.
type Recorded struct {
	Outcome Outcome
	Consent Consent
}

// Record records an observation of a Destination's consent in a transaction of its own; see
// RecordIn.
func (m *Module) Record(ctx context.Context, s *ent.Scoped, in RecordInput) (Recorded, error) {
	var out Recorded
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		var err error
		out, err = m.RecordIn(ctx, ts, in)
		return err
	})
	return out, err
}

// RecordIn records an observation of a Destination's consent inside the bus transaction s is bound
// to, and returns what it did with the consent that stands after it. Provenance is checked first:
// a `denied` applies from any provenance, a `granted` over a standing `denied` only from the
// person's own page, else ErrRefusalStands. Then time (consentstamps): an observation older than
// the one that set the consent changes nothing and is recorded as contact.consent_superseded for
// the Contact holding the Destination (Superseded), unless it agrees with the consent standing
// (a replay of the same observation, say), which records nothing at all. An observation that changes the state or the
// provenance writes the row and publishes its Channel's Event; one that repeats the consent
// standing only moves its stamp. Only the Channels open today take one (ErrChannel), and a grant
// in the future is ErrInFuture.
func (m *Module) RecordIn(ctx context.Context, s *ent.Scoped, in RecordInput) (Recorded, error) {
	in, err := m.checked(in)
	if err != nil {
		return Recorded{}, err
	}
	key := stampKey(in.Channel, in.Destination)
	if _, err := m.stamps.Hold(ctx, s, key); err != nil {
		return Recorded{}, err
	}
	standing, err := standingOf(ctx, s, in.Channel, in.Destination)
	if err != nil {
		return Recorded{}, err
	}
	if in.State == Granted && standing != nil && standing.State == Denied && in.Provenance != Page {
		return Recorded{}, ErrRefusalStands
	}
	admitted, err := m.stamps.Admit(ctx, s, consentstamps.Observation{
		Key: key, Fact: consentstamps.EngagementTracking, Occurred: consentstamps.At(in.OccurredAt), Refusal: in.State == Denied,
	})
	if err != nil {
		return Recorded{}, err
	}
	holder, err := m.holderOf(ctx, s, in.Channel, in.Destination)
	if err != nil {
		return Recorded{}, err
	}
	outcome := Applied
	switch {
	case !admitted && standing != nil && standing.State == in.State:
		// It agrees with the consent standing, set later or at the same moment (a replay): it
		// changes nothing and is no history.
	case !admitted:
		outcome = Superseded
		err = m.publishSuperseded(ctx, s, holder, in)
	case standing == nil || standing.State != in.State || standing.Provenance != in.Provenance:
		standing, err = m.write(ctx, s, standing, in, holder)
	}
	if err != nil {
		return Recorded{}, err
	}
	return Recorded{Outcome: outcome, Consent: consentOf(in.Channel, in.Destination, standing, defaultOf())}, nil
}

// checked is in normalized, or its refusal: an open Channel, the Destination as the Channel's
// Destinations are stored, a known state and provenance, and the time it occurred: now when it
// states none, and never after now (a grant dated later is ErrInFuture, a refusal is taken as now).
func (m *Module) checked(in RecordInput) (RecordInput, error) {
	if !open[in.Channel] {
		return RecordInput{}, ErrChannel
	}
	if err := validation.MaxLength(fieldname.Destination, in.Destination, bounds.ConsentDestinationMaxLength); err != nil {
		return RecordInput{}, err
	}
	dest, err := unsubscribes.Destination(in.Channel, in.Destination)
	if err != nil {
		return RecordInput{}, err
	}
	in.Destination = dest
	if engagementtrackingconsent.StateValidator(in.State) != nil {
		return RecordInput{}, errState
	}
	if engagementtrackingconsent.ProvenanceValidator(in.Provenance) != nil {
		return RecordInput{}, errProvenance
	}
	now := m.now()
	switch {
	case in.OccurredAt.IsZero():
		in.OccurredAt = now
	case in.OccurredAt.After(now) && in.State == Denied:
		// A refusal is never bounced for a clock ahead of ours: it is taken as occurring now.
		in.OccurredAt = now
	case in.OccurredAt.After(now):
		return RecordInput{}, ErrInFuture
	}
	return in, nil
}

// write sets the consent of in's Destination to in's state and provenance, over standing (nil
// when none is recorded), and publishes its Channel's Event naming holder, in the transaction s is
// bound to. The Destination's stamp is held, so nobody else writes the row meanwhile.
func (m *Module) write(ctx context.Context, s *ent.Scoped, standing *ent.EngagementTrackingConsent, in RecordInput, holder int64) (*ent.EngagementTrackingConsent, error) {
	var (
		row *ent.EngagementTrackingConsent
		err error
	)
	if standing == nil {
		row, err = s.EngagementTrackingConsent().Create().
			SetChannel(in.Channel).SetDestination(in.Destination).SetState(in.State).SetProvenance(in.Provenance).
			Save(ctx)
	} else {
		row, err = s.EngagementTrackingConsent().UpdateOneID(standing.ID).
			SetState(in.State).SetProvenance(in.Provenance).
			Save(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("record engagement tracking consent: %w", err)
	}
	fact := events.EngagementTracking{
		WorkspaceID: s.WorkspaceID(), ContactID: holder, Channel: in.Channel, Destination: in.Destination,
		Provenance: string(in.Provenance), OccurredAt: in.OccurredAt,
	}
	var proof events.DomainEvent = &events.EngagementTrackingGranted{EngagementTracking: fact}
	if in.State == Denied {
		proof = &events.EngagementTrackingDenied{EngagementTracking: fact}
	}
	if err := m.bus.PublishIn(ctx, s, proof); err != nil {
		return nil, err
	}
	return row, nil
}

// publishSuperseded records an observation older than the consent standing as
// contact.consent_superseded for the Contact holding its Destination (ADR 0154); a Destination no
// Contact holds has nobody whose history it joins.
func (m *Module) publishSuperseded(ctx context.Context, s *ent.Scoped, holder int64, in RecordInput) error {
	if holder == 0 {
		return nil
	}
	observation := events.ObservationTrackingGranted
	if in.State == Denied {
		observation = events.ObservationTrackingDenied
	}
	e := &events.ConsentSuperseded{
		WorkspaceID: s.WorkspaceID(), ContactID: holder, Channel: string(in.Channel),
		Observation: observation, Provenance: string(in.Provenance), ObservedAt: in.OccurredAt,
	}
	if in.Channel == channels.Email {
		e.Email = in.Destination
	}
	return m.bus.PublishIn(ctx, s, e)
}

// holderOf is the Contact holding destination on channel, locked against its Erasure to the end
// of the transaction (contactlock.Referenced), 0 when none does.
func (m *Module) holderOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (int64, error) {
	q := s.Contact().Query()
	switch channel {
	case channels.Email:
		q.Where(contact.Email(destination))
	case channels.SMS:
		q.Where(contact.Phone(destination))
	default:
		// A Telegram Destination is held through a Binding, which the sending context keeps.
		return 0, nil
	}
	id, err := q.OnlyID(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find the destination's contact: %w", err)
	}
	return m.locks.Referenced(ctx, s, id)
}

// ContactConsents is the consent of each Destination of a Contact.
type ContactConsents struct {
	Destinations []Consent
}

// ReadContact reads the consent of each Destination of a Contact: its email address. A Contact
// without one has none.
func (m *Module) ReadContact(ctx context.Context, s *ent.Scoped, contactID int64) (ContactConsents, error) {
	c, err := contactOf(ctx, s, contactID)
	if err != nil {
		return ContactConsents{}, err
	}
	out := ContactConsents{Destinations: []Consent{}}
	for _, d := range destinationsOf(c) {
		consent, err := m.Of(ctx, s, d.channel, d.address)
		if err != nil {
			return ContactConsents{}, err
		}
		out.Destinations = append(out.Destinations, consent)
	}
	return out, nil
}

// MemberInput is a member's observation, on the Contact card, of one of the Contact's
// Destinations: the person refused or agreed to tracking by another route (a call, a reply).
type MemberInput struct {
	Channel     channels.Channel
	Destination string
	State       State
}

// RecordByMember records a member's observation of one of a Contact's Destinations as manual,
// occurring now, in a transaction of its own, by the rules of RecordIn: a refusal always applies,
// a grant over a standing `denied` is ErrRefusalStands. A Destination the Contact does not hold is
// ErrNotTheContacts, and a Contact the Workspace does not have is ErrContactNotFound.
func (m *Module) RecordByMember(ctx context.Context, s *ent.Scoped, contactID int64, in MemberInput) (Recorded, error) {
	var out Recorded
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		c, err := contactOf(ctx, ts, contactID)
		if err != nil {
			return err
		}
		if !open[in.Channel] {
			return ErrChannel
		}
		address, err := unsubscribes.Destination(in.Channel, in.Destination)
		if err != nil {
			return err
		}
		if !slices.Contains(destinationsOf(c), destination{channel: in.Channel, address: address}) {
			return ErrNotTheContacts
		}
		out, err = m.RecordIn(ctx, ts, RecordInput{Channel: in.Channel, Destination: address, State: in.State, Provenance: Manual})
		return err
	})
	return out, err
}

// destination is one Destination of a Contact on a Channel, in its stored form.
type destination struct {
	channel channels.Channel
	address string
}

// destinationsOf is the Destinations of c that take a consent: its email address.
func destinationsOf(c *ent.Contact) []destination {
	if c.Email == nil {
		return nil
	}
	return []destination{{channel: channels.Email, address: *c.Email}}
}

// contactOf is the Contact contactID of the Workspace, or ErrContactNotFound.
func contactOf(ctx context.Context, s *ent.Scoped, contactID int64) (*ent.Contact, error) {
	c, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return nil, ErrContactNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read contact: %w", err)
	}
	return c, nil
}

// Of is the consent of destination on channel, stored as it is (the Channel's normalized form).
func (m *Module) Of(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (Consent, error) {
	row, err := standingOf(ctx, s, channel, destination)
	if err != nil {
		return Consent{}, err
	}
	return consentOf(channel, destination, row, defaultOf()), nil
}

// Observation is an observation of one Destination of a Contact, as a Contact intake row carries
// it: the state, and when it occurred (the zero time is now).
type Observation struct {
	State      State
	OccurredAt time.Time
}

// ContactObservations are the observations a Contact intake row carries, per Destination of the
// Contact; Email is nil when it carries none for the email address.
type ContactObservations struct {
	Email *Observation
}

// ImportIn records the observations an intake row carries for a Contact whose email address is
// email ("" when it has none), as imported, inside the bus transaction s is bound to: one for the
// email address of a Contact without one is ErrNoEmail, so an import row never drops it silently.
func (m *Module) ImportIn(ctx context.Context, s *ent.Scoped, email string, in ContactObservations) error {
	if in.Email == nil {
		return nil
	}
	if email == "" {
		return ErrNoEmail
	}
	_, err := m.RecordIn(ctx, s, RecordInput{
		Channel: channels.Email, Destination: email, State: in.Email.State, Provenance: Imported, OccurredAt: in.Email.OccurredAt,
	})
	return err
}

// Effective is whether a Destination whose consent is state (nil when none is recorded) is
// tracked under the Workspace's default dflt.
func Effective(state *State, dflt Decision) Decision {
	switch {
	case state == nil:
		return dflt
	case *state == Granted:
		return Track
	default:
		return DontTrack
	}
}

// defaultOf is the Workspace's Engagement tracking default: track until the Workspace setting
// exists, which then decides here.
func defaultOf() Decision { return Track }

// consentOf is the Consent of destination on channel whose row is row, nil when none is recorded.
func consentOf(channel channels.Channel, destination string, row *ent.EngagementTrackingConsent, dflt Decision) Consent {
	out := Consent{Channel: channel, Destination: destination, State: ReadingDefault, Effective: Effective(nil, dflt)}
	if row == nil {
		return out
	}
	out.State = Reading(row.State)
	out.Provenance = &row.Provenance
	out.Effective = Effective(&row.State, dflt)
	out.UpdatedAt = &row.UpdatedAt
	return out
}

// standingOf is the consent row of destination on channel, nil when none is recorded.
func standingOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (*ent.EngagementTrackingConsent, error) {
	row, err := s.EngagementTrackingConsent().Query().Where(
		engagementtrackingconsent.ChannelEQ(channel),
		engagementtrackingconsent.DestinationEQ(destination),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read engagement tracking consent: %w", err)
	}
	return row, nil
}

// stampKey is the consent stamp the consent of destination on channel is ordered by: the
// `everything` source, as a Confirmation's, since the consent has no Sending source.
func stampKey(channel channels.Channel, destination string) consentstamps.Key {
	return consentstamps.Key{Channel: channel, Destination: destination, Source: eligibility.SourceEverything}
}
