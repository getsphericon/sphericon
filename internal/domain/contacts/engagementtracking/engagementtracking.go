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

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/ent/workspace"
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
	"github.com/getsphericon/sphericon/internal/kernel/phone"
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

// Decision is whether a Destination's opens and clicks are tracked: the effective consent, and the
// Workspace's Engagement tracking default, which decides for a Destination with no consent.
type Decision = workspace.EngagementTrackingDefault

// The decisions: track, or don't.
const (
	Track     = workspace.EngagementTrackingDefaultTrack
	DontTrack = workspace.EngagementTrackingDefaultDontTrack
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
	// ErrChannel refuses a Channel whose messages track no engagement, so whose Destinations take
	// no consent.
	ErrChannel = validation.RefuseField(fieldname.Channel, "engagement tracking: only email, sms and telegram take a consent",
		i18n.ErrorsEngagementTrackingChannel)
	// ErrInFuture refuses a grant said to occur after now: it would outrank every real refusal.
	ErrInFuture = validation.RefuseField(fieldname.OccurredAt, "engagement tracking: occurredAt of a grant must not be in the future",
		i18n.ErrorsEngagementTrackingInFuture)
	// ErrNoEmail refuses a Contact's email consent for a Contact without an email address.
	ErrNoEmail = validation.RefuseField(fieldname.Email, "engagement tracking: the contact has no email", i18n.ErrorsContactEmailMissing)
	// ErrNoPhone refuses a Contact's SMS consent for a Contact without a phone.
	ErrNoPhone = validation.RefuseField(fieldname.Phone, "engagement tracking: the contact has no phone", i18n.ErrorsContactPhoneMissing)
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

// open is the Channels whose messages track engagement (ADR 0083), so whose Destinations take a
// consent: an email address, a phone number in E.164, the Telegram user id a Binding names.
var open = map[channels.Channel]bool{channels.Email: true, channels.SMS: true, channels.Telegram: true}

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
	dflt, err := defaultOf(ctx, s)
	if err != nil {
		return Recorded{}, err
	}
	return Recorded{Outcome: outcome, Consent: consentOf(in.Channel, in.Destination, standing, dflt)}, nil
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
	switch in.Channel {
	case channels.Email:
		e.Email = in.Destination
	case channels.SMS:
		e.Phone = in.Destination
	case channels.Telegram:
		// A Telegram user id has no column of the Event row: the Event names it by its Contact.
	}
	return m.bus.PublishIn(ctx, s, e)
}

// holderOf is the Contact holding destination on channel, locked against its Erasure to the end
// of the transaction (contactlock.Referenced), 0 when none does. A Telegram user id is held by the
// Contact of its first Binding, the one Contact the Event names; a Visitor's Binding reaches no
// Contact (ADR 0171).
func (m *Module) holderOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (int64, error) {
	var (
		id  int64
		err error
	)
	switch channel {
	case channels.Email:
		id, err = s.Contact().Query().Where(contact.Email(destination)).OnlyID(ctx)
	case channels.SMS:
		id, err = s.Contact().Query().Where(contact.Phone(destination)).OnlyID(ctx)
	case channels.Telegram:
		id, err = boundContactOf(ctx, s, destination)
	}
	switch {
	case ent.IsNotFound(err), err == nil && id == 0:
		return 0, nil
	case err != nil:
		return 0, fmt.Errorf("find the destination's contact: %w", err)
	}
	return m.locks.Referenced(ctx, s, id)
}

// boundContactOf is the Contact of the first Binding of a Telegram user id that is a Contact's, 0
// when none is.
func boundContactOf(ctx context.Context, s *ent.Scoped, user string) (int64, error) {
	b, err := s.Binding().Query().
		Where(binding.ChannelEQ(binding.ChannelTelegram), binding.Destination(user), binding.ContactIDNotNil()).
		Order(binding.ByBoundAt(), binding.ByID()).
		First(ctx)
	if err != nil {
		return 0, err
	}
	return lo.FromPtr(b.ContactID), nil
}

// ContactConsents is the consent of each Destination of a Contact.
type ContactConsents struct {
	Destinations []Consent
}

// ReadContact reads the consent of each Destination of a Contact: its email address, its phone,
// and the Telegram user of each of its chats. A Contact without any has none.
func (m *Module) ReadContact(ctx context.Context, s *ent.Scoped, contactID int64) (ContactConsents, error) {
	c, err := contactOf(ctx, s, contactID)
	if err != nil {
		return ContactConsents{}, err
	}
	held, err := destinationsOf(ctx, s, c)
	if err != nil {
		return ContactConsents{}, err
	}
	out := ContactConsents{Destinations: []Consent{}}
	for _, d := range held {
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
		held, err := destinationsOf(ctx, ts, c)
		if err != nil {
			return err
		}
		if !slices.Contains(held, destination{channel: in.Channel, address: address}) {
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

// destinationsOf is the Destinations of c that take a consent: its email address, its phone, and
// the Telegram users of its own Bindings on any bot of the Workspace, the chats its card lists and
// its sends reach (a Visitor's Binding reaches no Contact, ADR 0171).
func destinationsOf(ctx context.Context, s *ent.Scoped, c *ent.Contact) ([]destination, error) {
	var out []destination
	if c.Email != nil {
		out = append(out, destination{channel: channels.Email, address: *c.Email})
	}
	if c.Phone != nil {
		out = append(out, destination{channel: channels.SMS, address: *c.Phone})
	}
	users, err := s.Binding().Query().
		Where(
			binding.ChannelEQ(binding.ChannelTelegram),
			binding.ContactID(c.ID),
		).
		Unique(true).
		Order(binding.ByDestination()).
		Select(binding.FieldDestination).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the contact's telegram users: %w", err)
	}
	for _, user := range users {
		out = append(out, destination{channel: channels.Telegram, address: user})
	}
	return out, nil
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

// Of is the consent of destination on channel, stored as it is (the Channel's normalized form),
// and its effective consent under the Workspace's default as it stands now: the one answer for one
// Destination the send path and the recorder read.
func (m *Module) Of(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (Consent, error) {
	row, err := standingOf(ctx, s, channel, destination)
	if err != nil {
		return Consent{}, err
	}
	dflt, err := defaultOf(ctx, s)
	if err != nil {
		return Consent{}, err
	}
	return consentOf(channel, destination, row, dflt), nil
}

// Observation is an observation of one Destination of a Contact, as a Contact intake row carries
// it: the state, and when it occurred (the zero time is now).
type Observation struct {
	State      State
	OccurredAt time.Time
}

// ContactObservations are the observations a Contact intake row carries, per Destination of the
// Contact; Email is nil when it carries none for the email address, SMS when it carries none for
// the phone.
type ContactObservations struct {
	Email *Observation
	SMS   *Observation
}

// ImportInput is what a Contact intake row records: the Contact's email address and phone ("" when
// it has none), and the observations the row carries for them.
type ImportInput struct {
	Email        string
	Phone        string
	Observations ContactObservations
}

// ImportIn records the observations an intake row carries for a Contact, as imported, inside the
// bus transaction s is bound to: one for the email address of a Contact without one is ErrNoEmail,
// one for the phone of a Contact without one ErrNoPhone, so an import row never drops it silently.
func (m *Module) ImportIn(ctx context.Context, s *ent.Scoped, in ImportInput) error {
	if err := m.importOne(ctx, s, channels.Email, in.Email, in.Observations.Email, ErrNoEmail); err != nil {
		return err
	}
	return m.importOne(ctx, s, channels.SMS, in.Phone, in.Observations.SMS, ErrNoPhone)
}

// importOne records the observation o of destination on channel as imported; none is nothing, and
// one without a destination is missing.
func (m *Module) importOne(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string, o *Observation, missing error) error {
	if o == nil {
		return nil
	}
	if destination == "" {
		return missing
	}
	_, err := m.RecordIn(ctx, s, RecordInput{
		Channel: channel, Destination: destination, State: o.State, Provenance: Imported, OccurredAt: o.OccurredAt,
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

// defaultOf is the Engagement tracking default of the Workspace of s, as it stands in the
// transaction s is bound to.
func defaultOf(ctx context.Context, s *ent.Scoped) (Decision, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return "", fmt.Errorf("read the engagement tracking default: %w", err)
	}
	return ws.EngagementTrackingDefault, nil
}

// TrackedWhere is the effective consent in SQL, over many Destinations at once: it keeps the rows
// of the table it is applied to whose Destination, in their column (the stored, normalized form:
// a Contact's `email`, an Outbound message's `destination`), is tracked on channel under the
// Workspace's default dflt, matched in the row's own Workspace. It is Effective as a predicate: a
// Destination is tracked when its consent is granted, or when none is recorded and the default is
// to track. A NULL column has no consent: it reads as the default.
func TrackedWhere(channel channels.Channel, dflt Decision, column string) func(*sql.Selector) {
	return func(outer *sql.Selector) {
		outer.Where(trackedOf(outer, channel, dflt, column))
	}
}

// UntrackedWhere is TrackedWhere's complement: the rows whose Destination is not tracked on
// channel under dflt.
func UntrackedWhere(channel channels.Channel, dflt Decision, column string) func(*sql.Selector) {
	return func(outer *sql.Selector) {
		outer.Where(sql.Not(trackedOf(outer, channel, dflt, column)))
	}
}

// trackedOf is TrackedWhere's predicate over outer. With the default resolved here, one EXISTS
// decides: under track, a Destination is tracked unless a denied is recorded; under dont_track,
// only when a granted is.
func trackedOf(outer *sql.Selector, channel channels.Channel, dflt Decision, column string) *sql.Predicate {
	consents := sql.Table(engagementtrackingconsent.Table)
	state, exists := Granted, sql.Exists
	if dflt == Track {
		state, exists = Denied, sql.NotExists
	}
	sameDestination := sql.ColumnsEQ(consents.C(engagementtrackingconsent.FieldDestination), outer.C(column))
	if channel == channels.SMS {
		// On sms a refusal is kept by the number or, once its Contact was erased, by the number's
		// hash (phone.Hash, here in SQL, as the eligibility layers match an opt-out), so it holds
		// when the number comes back.
		// raw sql: the destination correlation, matched against the number or its SHA-256 hex.
		sameDestination = sql.P(func(b *sql.Builder) {
			b.WriteString(consents.C(engagementtrackingconsent.FieldDestination)).WriteString(" IN (").
				WriteString(outer.C(column)).WriteString(", encode(sha256(convert_to(").
				WriteString(outer.C(column)).WriteString(", 'UTF8')), 'hex'))")
		})
	}
	return exists(sql.Select(consents.C(engagementtrackingconsent.FieldID)).From(consents).Where(sql.And(
		// Every Workspace-owned table names its Workspace in the same column (WorkspaceMixin).
		sql.ColumnsEQ(consents.C(engagementtrackingconsent.FieldWorkspaceID), outer.C(engagementtrackingconsent.FieldWorkspaceID)),
		sameDestination,
		sql.EQ(consents.C(engagementtrackingconsent.FieldChannel), string(channel)),
		sql.EQ(consents.C(engagementtrackingconsent.FieldState), string(state)),
	)))
}

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

// standingOf is the consent row of destination on channel, nil when none is recorded. On sms the
// row is the number's or, once the Contact whose phone it was is erased, its hash's
// (engagementtrackingunchecked), never both: an observation of the number writes the hash's row
// when one stands, and an Erasure rekeys the number's only where no hash's stands.
func standingOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (*ent.EngagementTrackingConsent, error) {
	same := engagementtrackingconsent.DestinationEQ(destination)
	if channel == channels.SMS {
		same = engagementtrackingconsent.DestinationIn(destination, phone.Hash(destination))
	}
	row, err := s.EngagementTrackingConsent().Query().Where(
		engagementtrackingconsent.ChannelEQ(channel),
		same,
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
