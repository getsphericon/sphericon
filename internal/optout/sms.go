package optout

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/phone"
)

// EraseSMS is the SMS opt-out part of a Contact's Erasure (ADR 0021): the opt-outs of the Contact
// on sms and those of its number keep refusing the number, but keyed by its hash (phone.Hash)
// instead of the number. A hashed opt-out of the same scope that already stands (a person erased
// before with the number) makes the number's own redundant, so that one goes. number is "" for a
// Contact with no phone. Run it inside the Erasure's transaction, before the Contact reference is
// cleared.
func EraseSMS(ctx context.Context, s *ent.Scoped, contactID int64, number string) error {
	match := []predicate.Unsubscribe{unsubscribe.ContactID(contactID)}
	if number != "" {
		match = append(match, unsubscribe.DestinationEQ(number))
	}
	rows, err := s.Unsubscribe().Query().
		Where(unsubscribe.ChannelEQ(channels.SMS), unsubscribe.Or(match...)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("find sms opt-outs: %w", err)
	}
	for _, u := range rows {
		hashed := phone.Hash(u.Destination)
		twin, err := s.Unsubscribe().Query().Where(
			unsubscribe.ChannelEQ(channels.SMS),
			unsubscribe.DestinationEQ(hashed),
			unsubscribe.SendingSourceEQ(u.SendingSource),
		).Exist(ctx)
		if err != nil {
			return fmt.Errorf("find hashed opt-out: %w", err)
		}
		if twin {
			err = s.Unsubscribe().DeleteOneID(u.ID).Exec(ctx)
		} else {
			err = s.Unsubscribe().UpdateOneID(u.ID).SetDestination(hashed).Exec(ctx)
		}
		if err != nil {
			return fmt.Errorf("hash sms opt-out: %w", err)
		}
	}
	return nil
}

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
		SetChannel(channels.SMS).
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
		confirmation.ChannelEQ(channels.SMS),
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
	u, err := s.Unsubscribe().Query().Where(smsEverything(number)...).Order(ent.Asc(unsubscribe.FieldID)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find opt-out: %w", err)
	}
	return u, nil
}

// smsEverything is the Channel-wide opt-out of the number: kept by the number, or by its hash once
// the Contact whose phone it was is erased (EraseSMS), so a STOP, a START and the Contact card find
// an erased person's opt-out when the number comes back.
func smsEverything(number string) []predicate.Unsubscribe {
	return []predicate.Unsubscribe{
		unsubscribe.ChannelEQ(channels.SMS),
		unsubscribe.DestinationIn(number, phone.Hash(number)),
		unsubscribe.SendingSourceEQ(eligibility.SourceEverything),
	}
}
