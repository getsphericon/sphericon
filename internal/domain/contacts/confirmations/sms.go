package confirmations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Consent recorded from evidence (ADR 0077): a text goes only to a number with a Confirmation on
// sms and no opt-out there. sphericon sends no code or confirmation text of its own: the
// Confirmation is recorded from the evidence of consent the Workspace collected (imported), or from
// a START the person texts (keyword). Both the Confirmation and the opt-out are keyed by the number
// in E.164. On email the evidence records an imported Confirmation too (ADR 0138), beside the one
// the person's double opt-in click records.

var (
	// ErrConfirmationChannel: a Confirmation is recorded by hand or by the API only on email and sms.
	ErrConfirmationChannel = validation.RefuseField(fieldname.Channel, "consent: only email and sms take a recorded confirmation", i18n.ErrorsConfirmationChannel)
	// ErrWordingEmpty, ErrSourceEmpty, ErrBrandEmpty: the evidence lacks a part.
	ErrWordingEmpty = validation.RefuseField(fieldname.Wording, "consent: wording must not be empty", i18n.ErrorsConsentWordingEmpty)
	ErrSourceEmpty  = validation.RefuseField(fieldname.Source, "consent: source must not be empty", i18n.ErrorsConsentSourceEmpty)
	ErrBrandEmpty   = validation.RefuseField(fieldname.Brand, "consent: brand must not be empty", i18n.ErrorsConsentBrandEmpty)
	// ErrNoPhone: SMS consent or an SMS opt-out for a Contact that has no phone.
	ErrNoPhone = validation.RefuseField(fieldname.Phone, "consent: the contact has no phone", i18n.ErrorsContactPhoneMissing)
	// ErrNoEmail: email consent for a Contact that has no email address.
	ErrNoEmail = validation.RefuseField(fieldname.Email, "consent: the contact has no email", i18n.ErrorsContactEmailMissing)
	// ErrContactNotFound: no such Contact in the Workspace.
	ErrContactNotFound = validation.NotFound("consent: contact not found", i18n.ErrorsContactNotFound)
)

// ConfirmInput is consent a Destination gave outside sphericon on a Channel, with its evidence.
type ConfirmInput struct {
	Channel     channels.Channel
	Destination string
	Evidence    Evidence
}

// Confirm records consent a destination gave outside sphericon on email or sms, with its
// evidence, as an imported Confirmation (ADR 0077, ADR 0138), and returns the Confirmation that
// stands: a stronger one already on the destination stays. A Telegram chat is its own opt-in and
// takes none. It never lifts an opt-out (ADR 0121). The consent occurred when its evidence says,
// else now (occurred); one older than a withdrawal of the destination's consent is ErrSuperseded.
func (m *Module) Confirm(ctx context.Context, s *ent.Scoped, in ConfirmInput) (*ent.Confirmation, error) {
	destination, err := confirmable(in.Channel, in.Destination)
	if err != nil {
		return nil, err
	}
	var standing *ent.Confirmation
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		standing, err = m.confirmIn(ctx, ts, in.Channel, destination, in.Evidence)
		return err
	})
	if err != nil {
		return nil, err
	}
	return standing, nil
}

// confirmable is raw as the Destinations of channel are stored, refused on a Channel that takes no
// recorded Confirmation.
func confirmable(channel channels.Channel, raw string) (string, error) {
	// The Confirmation's row bounds its Destination: its owner checks the bound.
	if err := validation.MaxLength(fieldname.Destination, raw, bounds.ConsentDestinationMaxLength); err != nil {
		return "", err
	}
	switch channel {
	case channels.Email, channels.SMS:
		return unsubscribes.Destination(channel, raw)
	case channels.Telegram:
		// A Binding is Telegram's opt-in.
	}
	return "", ErrConfirmationChannel
}

// ConfirmSMSIn records an imported SMS Confirmation of number with its evidence inside the bus
// transaction s is bound to, and returns the Confirmation that stands (see Confirm).
func (m *Module) ConfirmSMSIn(ctx context.Context, s *ent.Scoped, number string, ev Evidence) (*ent.Confirmation, error) {
	number, err := unsubscribes.SMSNumber(number)
	if err != nil {
		return nil, err
	}
	return m.confirmIn(ctx, s, channels.SMS, number, ev)
}

// confirmIn records an imported Confirmation of the stored destination on channel with its
// evidence, given when occurred says, inside the bus transaction s is bound to. The Confirmation
// names the Contact whose address or phone the destination is, when there is one.
func (m *Module) confirmIn(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string, ev Evidence) (*ent.Confirmation, error) {
	ev, err := ev.checked()
	if err != nil {
		return nil, err
	}
	holder, err := holderOf(ctx, s, channel, destination)
	if err != nil {
		return nil, err
	}
	standing, _, err := m.recordIn(ctx, s, grant{
		channel: channel, destination: destination, provenance: confirmation.ProvenanceImported,
		evidence: &ev, contactID: holder, at: consentstamps.At(m.occurred(ev)),
	})
	return standing, err
}

// occurred is when the consent of ev was given (ADR 0154): the time its evidence states, or now
// when it states none or one later than now, so consent can never outrank a refusal yet to come.
func (m *Module) occurred(ev Evidence) time.Time {
	now := m.now()
	if ev.OccurredAt.IsZero() || ev.OccurredAt.After(now) {
		return now
	}
	return ev.OccurredAt
}

// ImportInput is the consent a Contact intake row carries for one Destination of its Contact: the
// Channel, the Contact's address or phone on it ("" when it has none) and the evidence.
type ImportInput struct {
	Channel     channels.Channel
	Destination string
	Evidence    Evidence
}

// ImportIn records the consent an intake row carries (ADR 0077, ADR 0138) as an imported
// Confirmation inside the bus transaction s is bound to. Consent for a Destination the Contact
// lacks is refused (ErrNoEmail, ErrNoPhone), so an import row never drops it silently. It lifts no
// opt-out, and a stronger Confirmation stands. Consent given before the Destination's consent was
// last withdrawn changes nothing and is no refusal of the row: it joins the Contact's history as
// contact.consent_superseded (ADR 0154).
func (m *Module) ImportIn(ctx context.Context, s *ent.Scoped, in ImportInput) error {
	if in.Destination == "" {
		return missingDestination(in.Channel)
	}
	destination, err := confirmable(in.Channel, in.Destination)
	if err != nil {
		return err
	}
	_, err = m.confirmIn(ctx, s, in.Channel, destination, in.Evidence)
	if !errors.Is(err, ErrSuperseded) {
		return err
	}
	holder, err := holderOf(ctx, s, in.Channel, destination)
	if err != nil || holder == 0 {
		return err
	}
	superseded := &events.ConsentSuperseded{
		WorkspaceID: s.WorkspaceID(), ContactID: holder, Channel: string(in.Channel),
		Observation: events.ObservationStart, Provenance: string(confirmation.ProvenanceImported),
		ObservedAt: m.occurred(in.Evidence),
	}
	switch in.Channel {
	case channels.Email:
		superseded.Email = destination
	case channels.SMS:
		superseded.Phone = destination
	case channels.Telegram:
		// confirmable refused it.
	}
	return m.bus.PublishIn(ctx, s, superseded)
}

// missingDestination is the refusal of consent for a Destination a Contact lacks on channel.
func missingDestination(channel channels.Channel) error {
	switch channel {
	case channels.Email:
		return ErrNoEmail
	case channels.SMS:
		return ErrNoPhone
	case channels.Telegram:
	}
	return ErrConfirmationChannel
}

// StartSMSIn reads a START the number texted, which occurred at the given time (ADR 0077): it lifts
// the number's opt-out and records a Confirmation with provenance keyword, since the person asked
// to receive again, inside the bus transaction s is bound to. A stronger Confirmation already on
// the number stays (recordIn), so a repeated START records nothing twice. The Confirmation names
// the Contact whose phone the number is, when there is one; a START creates no Contact. A START
// that occurred before the number's opt-out state (the person stopped later) lifts nothing,
// records nothing and reports false (ADR 0154).
func (m *Module) StartSMSIn(ctx context.Context, s *ent.Scoped, number string, at consentstamps.Occurrence) (bool, error) {
	number, err := unsubscribes.SMSNumber(number)
	if err != nil {
		return false, err
	}
	applied, err := m.unsubs.LiftSMS(ctx, s, number, at)
	if err != nil || !applied {
		return false, err
	}
	holder, err := holderOf(ctx, s, channels.SMS, number)
	if err != nil {
		return false, err
	}
	_, _, err = m.recordIn(ctx, s, grant{
		channel: channels.SMS, destination: number, provenance: confirmation.ProvenanceKeyword,
		contactID: holder, at: at,
	})
	if errors.Is(err, ErrSuperseded) {
		// The opt-out lifted, but a withdrawal later than the START keeps the number unconfirmed.
		return true, nil
	}
	return true, err
}

// holderOf is the id of the Contact whose address (on email) or phone (on sms) destination is, 0
// when no Contact has it. Telegram has no Confirmation: ErrConfirmationChannel.
func holderOf(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string) (int64, error) {
	var holds predicate.Contact
	switch channel {
	case channels.Email:
		holds = contact.Email(destination)
	case channels.SMS:
		holds = contact.Phone(destination)
	case channels.Telegram:
		return 0, ErrConfirmationChannel
	}
	id, err := s.Contact().Query().Where(holds).OnlyID(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find the destination's contact: %w", err)
	}
	return id, nil
}

// ContactSMS is whether a Contact's phone may be texted: its SMS Confirmation and its SMS opt-out,
// both read by the Contact's current phone. A Contact without a phone has neither.
type ContactSMS struct {
	Phone        *string
	Confirmation *ent.Confirmation
	OptOut       *SMSOptOut
}

// SMSOptOut is the opt-out of a phone from every SMS of the Workspace: how the person stopped, and
// when.
type SMSOptOut struct {
	Via        unsubscribe.Via
	OptedOutAt time.Time
}

// ReadContactSMS reads the SMS consent of a Contact's phone.
func (m *Module) ReadContactSMS(ctx context.Context, s *ent.Scoped, contactID int64) (ContactSMS, error) {
	c, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return ContactSMS{}, ErrContactNotFound
	}
	if err != nil {
		return ContactSMS{}, fmt.Errorf("read contact: %w", err)
	}
	if c.Phone == nil {
		return ContactSMS{}, nil
	}
	out := ContactSMS{Phone: c.Phone}
	out.Confirmation, err = s.Confirmation().Query().Where(
		confirmation.ChannelEQ(channels.SMS),
		confirmation.DestinationEQ(*c.Phone),
	).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return ContactSMS{}, fmt.Errorf("read confirmation: %w", err)
	}
	stop, err := m.unsubs.SMSOptOut(ctx, s, *c.Phone)
	if err != nil {
		return ContactSMS{}, err
	}
	if stop != nil {
		// Every SMS opt-out is recorded with its way (unsubscribes' AddSMS).
		out.OptOut = &SMSOptOut{Via: lo.FromPtr(stop.Via), OptedOutAt: stop.CreatedAt}
	}
	return out, nil
}

// ConfirmContactSMS records consent a Contact's phone gave outside sphericon (Confirm, for the
// number the Contact has) and returns the phone's SMS consent after it.
func (m *Module) ConfirmContactSMS(ctx context.Context, s *ent.Scoped, contactID int64, ev Evidence) (ContactSMS, error) {
	number, err := m.ContactPhone(ctx, s, contactID)
	if err != nil {
		return ContactSMS{}, err
	}
	if _, err := m.Confirm(ctx, s, ConfirmInput{Channel: channels.SMS, Destination: number, Evidence: ev}); err != nil {
		return ContactSMS{}, err
	}
	return m.ReadContactSMS(ctx, s, contactID)
}

// ContactPhone is the phone of a Contact, ErrNoPhone when it has none.
func (m *Module) ContactPhone(ctx context.Context, s *ent.Scoped, contactID int64) (string, error) {
	read, err := m.ReadContactSMS(ctx, s, contactID)
	if err != nil {
		return "", err
	}
	if read.Phone == nil {
		return "", ErrNoPhone
	}
	return *read.Phone, nil
}

// checked is the evidence trimmed, refused when a part is empty.
func (ev Evidence) checked() (Evidence, error) {
	out := Evidence{
		Wording:    strings.TrimSpace(ev.Wording),
		Source:     strings.TrimSpace(ev.Source),
		Brand:      strings.TrimSpace(ev.Brand),
		OccurredAt: ev.OccurredAt,
	}
	switch {
	case out.Wording == "":
		return Evidence{}, ErrWordingEmpty
	case out.Source == "":
		return Evidence{}, ErrSourceEmpty
	case out.Brand == "":
		return Evidence{}, ErrBrandEmpty
	}
	for _, f := range []struct {
		field, value string
		lo, hi       int
	}{
		{"wording", out.Wording, bounds.ConsentWordingMinLength, bounds.ConsentWordingMaxLength},
		{"source", out.Source, bounds.ConsentSourceMinLength, bounds.ConsentSourceMaxLength},
		{"brand", out.Brand, bounds.ConsentBrandMinLength, bounds.ConsentBrandMaxLength},
	} {
		if err := validation.Length(f.field, f.value, f.lo, f.hi); err != nil {
			return Evidence{}, err
		}
	}
	return out, nil
}
