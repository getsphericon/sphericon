package channelstop

import (
	"context"
	"fmt"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrSMSSource refuses a hand-recorded SMS opt-out of a narrower source: it is Channel-wide.
var ErrSMSSource = validation.RefuseField(fieldname.SendingSource, "consent: an sms opt-out is for everything", i18n.ErrorsSendingSourceSms)

// stopSMS records the Channel-wide opt-out of a phone number in E.164 (scope `everything`, Channel
// `sms`): no text of the Workspace goes to it, whichever sender, Broadcast or Automation (ADR 0077).
// Like an opt-out of everything on email it deletes the number's SMS Confirmation, so stale consent
// never reactivates: only a new Confirmation (the START that lifts the opt-out) makes the number
// reachable again; the sms.confirmed Event stays as proof. It returns the id of the opt-out it
// wrote, 0 when the number had one already: that changes nothing, and keeps the way it first
// stopped. via is how the person stopped, which the Contact card shows, and at when: a stop older
// than the number's opt-out state is superseded and changes nothing (ADR 0154). Run it inside the
// transaction s is bound to.
func (m *Module) stopSMS(ctx context.Context, s *ent.Scoped, number string, via unsubscribe.Via, at consentstamps.Occurrence) (unsubscribes.Change, error) {
	change, err := m.unsubs.AddSMS(ctx, s, unsubscribes.AddSMSInput{Number: number, Via: via, At: at})
	if err != nil || change.ID == 0 {
		return change, err
	}
	if err := m.confirmations.Withdraw(ctx, s, confirmations.WithdrawInput{Channel: channels.SMS, Destination: number, At: at}); err != nil {
		return unsubscribes.Change{}, fmt.Errorf("drop confirmation: %w", err)
	}
	return change, nil
}

// publishSMSUnsubscribed records sms.unsubscribed for the Contact whose phone the number is (a
// number is one Contact's at most in a Workspace); a number no Contact has gets none. The Event is
// keyed by the opt-out it reports: a new stop after a START is a new Event. Only a person following
// the unsubscribe link of a text they received names its Broadcast or Automation (ADR 0115), and
// only on the Event of the Contact the text was sent to: a number that has moved to another Contact
// since records that Contact's opt-out, naming none. A STOP, a refused send or an opt-out by hand
// names none either.
func (m *Module) publishSMSUnsubscribed(ctx context.Context, s *ent.Scoped, stop *events.SmsStopObserved, number string, optOutID int64) error {
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
	return m.bus.PublishIn(ctx, s, e)
}

// attributesSMSStop reports whether the stop is a person's action on a text they received: only
// that attributes the opt-out to the text's Broadcast or Automation (ADR 0115).
func attributesSMSStop(stop *events.SmsStopObserved) bool {
	return stop.Via == events.UnsubscribedByLink && stop.OutboundMessageID != 0
}

// HandStop is an opt-out of a Destination that reached the Workspace by another route than its
// Channel (an email, a call, another platform): recorded by hand through the app or the API
// (events.UnsubscribedByManual), or brought by an import (events.UnsubscribedByImported, ADR 0138).
type HandStop struct {
	Destination, Source string
	// Via is how the opt-out was made: one of events.UnsubscribedByManual and
	// events.UnsubscribedByImported.
	Via string
	// At is when the person stopped (ADR 0154); the zero time is now, by the module's clock.
	At time.Time
}

// SMSUnsubscribe records an SMS opt-out that reached the Workspace by another route: the number
// gets no more texts, whichever sender. Such an opt-out is Channel-wide, so only the `everything`
// source is accepted. Idempotent.
func (m *Module) SMSUnsubscribe(ctx context.Context, s *ent.Scoped, in HandStop) (*ent.Unsubscribe, error) {
	number, err := unsubscribes.SMSNumber(in.Destination)
	if err != nil {
		return nil, err
	}
	if in.Source != eligibility.SourceEverything {
		return nil, ErrSMSSource
	}
	stop := &events.SmsStopObserved{Phone: number, Via: in.Via, ObservedAt: in.At}
	if err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		stop.WorkspaceID = ts.WorkspaceID()
		return m.bus.PublishIn(ctx, ts, stop)
	}); err != nil {
		return nil, err
	}
	return m.unsubs.SMSOptOut(ctx, s, number)
}

// OptOutContactSMS records an SMS opt-out of a Contact's phone that reached the Workspace by
// another route by hand (SMSUnsubscribe) and returns the phone's SMS consent after it.
func (m *Module) OptOutContactSMS(ctx context.Context, s *ent.Scoped, contactID int64) (confirmations.ContactSMS, error) {
	number, err := m.confirmations.ContactPhone(ctx, s, contactID)
	if err != nil {
		return confirmations.ContactSMS{}, err
	}
	stop := HandStop{Destination: number, Source: eligibility.SourceEverything, Via: events.UnsubscribedByManual}
	if _, err := m.SMSUnsubscribe(ctx, s, stop); err != nil {
		return confirmations.ContactSMS{}, err
	}
	return m.confirmations.ReadContactSMS(ctx, s, contactID)
}
