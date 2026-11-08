package channelstop

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
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

// SMSStop is a phone number stopping the Workspace's texts.
type SMSStop struct {
	// Number is the phone number in E.164: the opt-out is keyed by it, so it covers every sender.
	Number string
	// Via is how the person stopped: one of events.UnsubscribedByKeyword, ByLink, ByRefusedSend or
	// ByManual, which are the opt-out row's own values.
	Via string
	// IntegrationID is the sender the stop came through, 0 when none did (a link, by hand).
	IntegrationID int64
	// ContactID is the Contact of the text the stop was made from (the text whose link was clicked,
	// the send the provider refused), 0 when no text is known (a keyword, by hand). BroadcastID and
	// AutomationID are where that text came from: they are named on its Contact's event, from which
	// the Broadcast counts the opt-out (the broadcasts consumer, internal/jobs).
	ContactID, BroadcastID, AutomationID int64
}

// StopSMS records the Channel-wide opt-out of a phone number (scope `everything`, Channel `sms`,
// the number in E.164): no text of the Workspace goes to it, whichever sender, Broadcast or
// Automation (ADR 0077). Like an opt-out of everything on email it deletes the number's SMS
// Confirmation, so stale consent never reactivates: only a new Confirmation (the START that lifts
// the opt-out) makes the number reachable again; the sms.confirmed Event stays as proof. It records
// sms.unsubscribed for the Contact whose phone the number is and for the Contact of the text the
// stop was made from, when that is another (ADR 0113). It is idempotent: a number already opted out
// (a repeated or concurrent stop) changes nothing, keeps the way it first stopped and records no
// second event. Run it inside the transaction s and pub are bound to.
func StopSMS(ctx context.Context, s *ent.Scoped, pub events.Publisher, st SMSStop) error {
	// The ways of stopping (events.UnsubscribedBy*) are the row's own values.
	row, err := unsubscribes.AddSMS(ctx, s, st.Number, unsubscribe.Via(st.Via))
	if err != nil || row == nil {
		return err
	}
	if err := confirmations.Withdraw(ctx, s, channels.SMS, st.Number); err != nil {
		return fmt.Errorf("drop confirmation: %w", err)
	}
	var contactIDs []int64
	if st.ContactID != 0 {
		contactIDs = append(contactIDs, st.ContactID)
	}
	if holder := lo.FromPtr(row.ContactID); holder != 0 && holder != st.ContactID {
		contactIDs = append(contactIDs, holder)
	}
	for _, id := range contactIDs {
		c, err := s.Contact().Get(ctx, id)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read contact: %w", err)
		}
		// The opt-out row is new with every stop (a START deletes it), so its id keys the event
		// without the number, which an Erasure must not leave behind in the Event's source id.
		e := &events.SmsUnsubscribed{
			WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), Phone: st.Number,
			IntegrationID: st.IntegrationID, Via: st.Via,
			DedupID: fmt.Sprintf("%s:%d/%d", events.NameSmsUnsubscribed, row.ID, c.ID),
		}
		if c.ID == st.ContactID {
			// The Broadcast counts the opt-out when its consumer works this event (broadcasts owns
			// the recipient row, ADR 0102): the job commits with the opt-out.
			e.BroadcastID, e.AutomationID = st.BroadcastID, st.AutomationID
		}
		if err := pub.Publish(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// SMSUnsubscribe records an SMS opt-out that reached the Workspace by another route (an email, a
// call): the number gets no more texts, whichever sender (StopSMS). A manual opt-out is
// Channel-wide, so only the `everything` source is accepted. Idempotent.
func SMSUnsubscribe(ctx context.Context, bus *events.Bus, s *ent.Scoped, destination, source string) (*ent.Unsubscribe, error) {
	number, err := unsubscribes.SMSNumber(destination)
	if err != nil {
		return nil, err
	}
	if source != eligibility.SourceEverything {
		return nil, ErrSMSSource
	}
	if err := bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		return StopSMS(ctx, ts, pub, SMSStop{Number: number, Via: events.UnsubscribedByManual})
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
