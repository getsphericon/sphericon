package consent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/optout"
)

// SMS consent (ADR 0077): a text goes only to a number with a Confirmation on sms and no opt-out
// there. sphericon sends no code or confirmation text of its own: the Confirmation is recorded from
// the evidence of consent the Workspace collected (imported), or from a START the person texts
// (keyword). Both the Confirmation and the opt-out are keyed by the number in E.164.

var (
	// ErrConfirmationChannel: a Confirmation is recorded by hand or by the API only on sms.
	ErrConfirmationChannel = validation.RefuseField("channel", "consent: only sms takes a recorded confirmation", i18n.ErrorsConfirmationChannel)
	// ErrWordingEmpty, ErrSourceEmpty, ErrBrandEmpty: the evidence lacks a part.
	ErrWordingEmpty = validation.RefuseField("wording", "consent: wording must not be empty", i18n.ErrorsConsentWordingEmpty)
	ErrSourceEmpty  = validation.RefuseField("source", "consent: source must not be empty", i18n.ErrorsConsentSourceEmpty)
	ErrBrandEmpty   = validation.RefuseField("brand", "consent: brand must not be empty", i18n.ErrorsConsentBrandEmpty)
	// ErrNoPhone: SMS consent or an SMS opt-out for a Contact that has no phone.
	ErrNoPhone = validation.RefuseField("phone", "consent: the contact has no phone", i18n.ErrorsContactPhoneMissing)
	// ErrSMSSource: an SMS opt-out recorded by hand is Channel-wide.
	ErrSMSSource = validation.RefuseField("sendingSource", "consent: an sms opt-out is for everything", i18n.ErrorsSendingSourceSms)
	// ErrContactNotFound: no such Contact in the Workspace.
	ErrContactNotFound = validation.NotFound("consent: contact not found", i18n.ErrorsContactNotFound)
)

// Confirm records consent a destination gave outside sphericon on channel, with its evidence, as
// an imported Confirmation, and returns the Confirmation that stands: a stronger one already on
// the destination stays. Only sms takes one: email is confirmed by its own double opt-in link and a
// Telegram chat is its own opt-in. It never lifts an opt-out.
func Confirm(ctx context.Context, bus *events.Bus, s *ent.Scoped, channel channels.Channel, destination string, ev Evidence) (*ent.Confirmation, error) {
	if channel != channels.SMS {
		return nil, ErrConfirmationChannel
	}
	number, err := smsNumber(destination)
	if err != nil {
		return nil, err
	}
	err = settled(bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		_, err := ConfirmSMSIn(ctx, ts, pub, number, ev)
		return err
	}))
	if err != nil {
		return nil, err
	}
	// Read after the commit, so a concurrent request that recorded the number first answers with
	// the Confirmation that stands.
	standing, err := s.Confirmation().Query().Where(
		confirmation.ChannelEQ(channels.SMS),
		confirmation.DestinationEQ(number),
	).Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("read confirmation: %w", err)
	}
	return standing, nil
}

// ConfirmSMSIn records an imported SMS Confirmation of number with its evidence, inside the
// transaction s and pub are bound to, and returns the Confirmation that stands (see Confirm). The
// Confirmation names the Contact whose phone the number is, when there is one.
func ConfirmSMSIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, number string, ev Evidence) (*ent.Confirmation, error) {
	number, err := smsNumber(number)
	if err != nil {
		return nil, err
	}
	ev, err = ev.checked()
	if err != nil {
		return nil, err
	}
	holder, err := holderOf(ctx, s, number)
	if err != nil {
		return nil, err
	}
	return recordIn(ctx, s, pub, grant{
		channel: channels.SMS, destination: number, provenance: confirmation.ProvenanceImported,
		evidence: &ev, contactID: holder,
	})
}

// StartSMSIn reads a START the number texted (ADR 0077): it lifts the number's opt-out and records a
// Confirmation with provenance keyword, since the person asked to receive again, inside the
// transaction s and pub are bound to. A stronger Confirmation already on the number stays (recordIn),
// so a repeated START records nothing twice. The Confirmation names the Contact whose phone the
// number is, when there is one; a START creates no Contact.
func StartSMSIn(ctx context.Context, s *ent.Scoped, pub events.Publisher, number string) error {
	if err := optout.LiftSMS(ctx, s, number); err != nil {
		return err
	}
	holder, err := holderOf(ctx, s, number)
	if err != nil {
		return err
	}
	_, err = recordIn(ctx, s, pub, grant{
		channel: channels.SMS, destination: number, provenance: confirmation.ProvenanceKeyword,
		contactID: holder,
	})
	return err
}

// holderOf is the id of the Contact whose phone number is, 0 when no Contact has it.
func holderOf(ctx context.Context, s *ent.Scoped, number string) (int64, error) {
	id, err := s.Contact().Query().Where(contact.Phone(number)).OnlyID(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find the number's contact: %w", err)
	}
	return id, nil
}

// SMSUnsubscribe records an SMS opt-out that reached the Workspace by another route (an email, a
// call): the number gets no more texts, whichever sender (optout.StopSMS). A manual opt-out is
// Channel-wide, so only the `everything` source is accepted. Idempotent.
func SMSUnsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	number, err := smsNumber(destination)
	if err != nil {
		return nil, err
	}
	if source != eligibility.SourceEverything {
		return nil, ErrSMSSource
	}
	if err := settled(bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return optout.StopSMS(ctx, ts, number, unsubscribe.ViaManual)
	})); err != nil {
		return nil, err
	}
	return optout.SMSOptOut(ctx, s, number)
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
func ReadContactSMS(ctx context.Context, s *ent.Scoped, contactID int64) (ContactSMS, error) {
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
	stop, err := optout.SMSOptOut(ctx, s, *c.Phone)
	if err != nil {
		return ContactSMS{}, err
	}
	if stop != nil {
		// Every SMS opt-out is recorded with its way (optout.StopSMS).
		out.OptOut = &SMSOptOut{Via: lo.FromPtr(stop.Via), OptedOutAt: stop.CreatedAt}
	}
	return out, nil
}

// ConfirmContactSMS records consent a Contact's phone gave outside sphericon (Confirm, for the
// number the Contact has) and returns the phone's SMS consent after it.
func ConfirmContactSMS(ctx context.Context, bus *events.Bus, s *ent.Scoped, contactID int64, ev Evidence) (ContactSMS, error) {
	number, err := phoneOf(ctx, s, contactID)
	if err != nil {
		return ContactSMS{}, err
	}
	if _, err := Confirm(ctx, bus, s, channels.SMS, number, ev); err != nil {
		return ContactSMS{}, err
	}
	return ReadContactSMS(ctx, s, contactID)
}

// OptOutContactSMS records an SMS opt-out of a Contact's phone that reached the Workspace by
// another route (SMSUnsubscribe) and returns the phone's SMS consent after it.
func OptOutContactSMS(ctx context.Context, bus *events.Bus, s *ent.Scoped, contactID int64) (ContactSMS, error) {
	number, err := phoneOf(ctx, s, contactID)
	if err != nil {
		return ContactSMS{}, err
	}
	if _, err := SMSUnsubscribe(ctx, bus, s, number, eligibility.SourceEverything); err != nil {
		return ContactSMS{}, err
	}
	return ReadContactSMS(ctx, s, contactID)
}

func phoneOf(ctx context.Context, s *ent.Scoped, contactID int64) (string, error) {
	read, err := ReadContactSMS(ctx, s, contactID)
	if err != nil {
		return "", err
	}
	if read.Phone == nil {
		return "", ErrNoPhone
	}
	return *read.Phone, nil
}

// smsNumber is a destination on sms: the phone number in E.164, which must carry its country code
// (a Confirmation or an opt-out has no Contact country to read a local number with).
func smsNumber(destination string) (string, error) {
	if strings.TrimSpace(destination) == "" {
		return "", ErrDestinationEmpty
	}
	number, err := phone.Normalize(destination, "")
	if err != nil {
		return "", ErrDestinationInvalid
	}
	return number, nil
}

// checked is the evidence trimmed, refused when a part is empty.
func (ev Evidence) checked() (Evidence, error) {
	out := Evidence{
		Wording: strings.TrimSpace(ev.Wording),
		Source:  strings.TrimSpace(ev.Source),
		Brand:   strings.TrimSpace(ev.Brand),
	}
	switch {
	case out.Wording == "":
		return Evidence{}, ErrWordingEmpty
	case out.Source == "":
		return Evidence{}, ErrSourceEmpty
	case out.Brand == "":
		return Evidence{}, ErrBrandEmpty
	}
	return out, nil
}
