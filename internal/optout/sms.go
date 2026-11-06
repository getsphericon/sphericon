package optout

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/confirmations"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/unsubscribes"
)

// ErrSMSSource refuses a hand-recorded SMS opt-out of a narrower source: it is Channel-wide.
var ErrSMSSource = validation.RefuseField("sendingSource", "consent: an sms opt-out is for everything", i18n.ErrorsSendingSourceSms)

// StopSMS records the Channel-wide opt-out of a phone number (scope `everything`, Channel `sms`,
// the number in E.164): no text of the Workspace goes to it, whichever sender, Broadcast or
// Automation (ADR 0077). Like an opt-out of everything on email it deletes the number's SMS
// Confirmation, so stale consent never reactivates: only a new Confirmation (the START that lifts
// the opt-out) makes the number reachable again; the sms.confirmed Event stays as proof. It
// is idempotent: a number already opted out changes nothing, and keeps the way it first stopped.
// via is how the person stopped, which the Contact card shows. Run it inside the transaction s is
// bound to.
func StopSMS(ctx context.Context, s *ent.Scoped, number string, via unsubscribe.Via) error {
	added, err := unsubscribes.AddSMS(ctx, s, number, via)
	if err != nil || !added {
		return err
	}
	if err := confirmations.Withdraw(ctx, s, channels.SMS, number); err != nil {
		return fmt.Errorf("drop confirmation: %w", err)
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
	if err := settled(bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return StopSMS(ctx, ts, number, unsubscribe.ViaManual)
	})); err != nil {
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

// settled reports a write that lost to a database constraint as done: the Workspace
// is gone (nothing is left to record) or a concurrent request recorded the same row
// first. Any other failure is a real one and is returned.
func settled(err error) error {
	if ent.IsConstraintError(err) {
		return nil
	}
	return err
}
