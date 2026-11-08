package channelstop

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrSMSSource refuses a hand-recorded SMS opt-out of a narrower source: it is Channel-wide.
var ErrSMSSource = validation.RefuseField("sendingSource", "consent: an sms opt-out is for everything", i18n.ErrorsSendingSourceSms)

// stopSMS records the Channel-wide opt-out of a phone number in E.164 (scope `everything`, Channel
// `sms`): no text of the Workspace goes to it, whichever sender, Broadcast or Automation (ADR 0077).
// Like an opt-out of everything on email it deletes the number's SMS Confirmation, so stale consent
// never reactivates: only a new Confirmation (the START that lifts the opt-out) makes the number
// reachable again; the sms.confirmed Event stays as proof. It returns the id of the opt-out it
// wrote, 0 when the number had one already: that changes nothing, and keeps the way it first
// stopped. via is how the person stopped, which the Contact card shows. Run it inside the
// transaction s is bound to.
func stopSMS(ctx context.Context, s *ent.Scoped, number string, via unsubscribe.Via) (int64, error) {
	optOutID, err := unsubscribes.AddSMS(ctx, s, number, via)
	if err != nil || optOutID == 0 {
		return 0, err
	}
	if err := confirmations.Withdraw(ctx, s, channels.SMS, number); err != nil {
		return 0, fmt.Errorf("drop confirmation: %w", err)
	}
	return optOutID, nil
}

// publishSMSUnsubscribed records sms.unsubscribed for the Contact whose phone the number is (a
// number is one Contact's at most in a Workspace); a number no Contact has gets none. The Event is
// keyed by the opt-out it reports: a new stop after a START is a new Event. Only a person following
// the unsubscribe link of a text they received names its Broadcast or Automation (ADR 0115), and
// only on the Event of the Contact the text was sent to: a number that has moved to another Contact
// since records that Contact's opt-out, naming none. A STOP, a refused send or an opt-out by hand
// names none either.
func publishSMSUnsubscribed(ctx context.Context, s *ent.Scoped, pub events.Publisher, stop *events.SmsStopObserved, number string, optOutID int64) error {
	c, err := s.Contact().Query().Where(contact.Phone(number)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the number's contact: %w", err)
	}
	e := &events.SmsUnsubscribed{
		WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Phone: number,
		Via: stop.Via, DedupID: fmt.Sprintf("opt-out:%d", optOutID),
	}
	if attributesSMSStop(stop) && stop.ContactID == c.ID {
		e.BroadcastID, e.AutomationID = stop.BroadcastID, stop.AutomationID
	}
	return pub.Publish(ctx, e)
}

// attributesSMSStop reports whether the stop is a person's action on a text they received: only
// that attributes the opt-out to the text's Broadcast or Automation (ADR 0115).
func attributesSMSStop(stop *events.SmsStopObserved) bool {
	return stop.Via == events.UnsubscribedByLink && stop.OutboundMessageID != 0
}

// SMSUnsubscribe records an SMS opt-out that reached the Workspace by another route (an email, a
// call): the number gets no more texts, whichever sender. A manual opt-out is
// Channel-wide, so only the `everything` source is accepted. Idempotent.
func SMSUnsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	number, err := unsubscribes.SMSNumber(destination)
	if err != nil {
		return nil, err
	}
	if source != eligibility.SourceEverything {
		return nil, ErrSMSSource
	}
	if err := bus.WithinScopedTx(ctx, s, func(_ *ent.Scoped, pub events.Publisher) error {
		return pub.Publish(ctx, &events.SmsStopObserved{WorkspaceID: s.WorkspaceID(), Phone: number, Via: events.UnsubscribedByManual})
	}); err != nil {
		return nil, err
	}
	return unsubscribes.SMSOptOut(ctx, s, number)
}

// OptOutContactSMS records an SMS opt-out of a Contact's phone that reached the Workspace by
// another route (SMSUnsubscribe) and returns the phone's SMS consent after it.
func OptOutContactSMS(ctx context.Context, bus *events.Bus, s *ent.Scoped, contactID int64) (confirmations.ContactSMS, error) {
	number, err := confirmations.ContactPhone(ctx, s, contactID)
	if err != nil {
		return confirmations.ContactSMS{}, err
	}
	if _, err := SMSUnsubscribe(ctx, bus, s, number, eligibility.SourceEverything); err != nil {
		return confirmations.ContactSMS{}, err
	}
	return confirmations.ReadContactSMS(ctx, s, contactID)
}
