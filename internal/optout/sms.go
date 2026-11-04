package optout

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
)

// StopSMS records the Channel-wide opt-out of a phone number (scope `everything`, Channel `sms`,
// the number in E.164): no text of the Workspace goes to it, whichever sender, Broadcast or
// Automation (ADR 0077). Like an opt-out of everything on email it deletes the number's SMS
// Confirmation, so stale consent never reactivates: only a new Confirmation (the START that lifts
// the opt-out) makes the number reachable again; the sms.confirmed Event stays as proof. It
// is idempotent: a number already opted out changes nothing, and keeps the way it first stopped.
// via is how the person stopped, which the Contact card shows. Run it inside the transaction s is
// bound to.
func StopSMS(ctx context.Context, s *ent.Scoped, number string, via unsubscribe.Via) error {
	exists, err := s.Unsubscribe().Query().Where(smsEverything(number)...).Exist(ctx)
	if err != nil {
		return fmt.Errorf("find opt-out: %w", err)
	}
	if exists {
		return nil
	}
	create := s.Unsubscribe().Create().
		SetChannel(unsubscribe.ChannelSms).
		SetDestination(number).
		SetSendingSource(eligibility.SourceEverything).
		SetVia(via)
	holder, err := s.Contact().Query().Where(contact.Phone(number)).OnlyID(ctx)
	switch {
	case err == nil:
		create.SetContactID(holder)
	case !ent.IsNotFound(err):
		return fmt.Errorf("find the number's contact: %w", err)
	}
	if err := create.Exec(ctx); err != nil {
		return fmt.Errorf("record opt-out: %w", err)
	}
	if _, err := s.Confirmation().Delete().Where(
		confirmation.ChannelEQ(confirmation.ChannelSms),
		confirmation.DestinationEQ(number),
	).Exec(ctx); err != nil {
		return fmt.Errorf("drop confirmation: %w", err)
	}
	return nil
}

// LiftSMS lifts the Channel-wide opt-out of a phone number: the person texted START. Nothing to
// lift is a no-op. It records no consent: the START's Confirmation is the caller's to record.
func LiftSMS(ctx context.Context, s *ent.Scoped, number string) error {
	if _, err := s.Unsubscribe().Delete().Where(smsEverything(number)...).Exec(ctx); err != nil {
		return fmt.Errorf("lift opt-out: %w", err)
	}
	return nil
}

// SMSOptOut is the Channel-wide opt-out of a phone number, nil when it has none.
func SMSOptOut(ctx context.Context, s *ent.Scoped, number string) (*ent.Unsubscribe, error) {
	u, err := s.Unsubscribe().Query().Where(smsEverything(number)...).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find opt-out: %w", err)
	}
	return u, nil
}

func smsEverything(number string) []predicate.Unsubscribe {
	return []predicate.Unsubscribe{
		unsubscribe.ChannelEQ(unsubscribe.ChannelSms),
		unsubscribe.DestinationEQ(number),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	}
}
