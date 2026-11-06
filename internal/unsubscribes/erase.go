package unsubscribes

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// The Unsubscribe parts of a Contact's Erasure (ADR 0021). Each runs inside the Erasure's
// transaction (s comes from it).

// EraseSMS is the SMS opt-out part of a Contact's Erasure: the opt-outs of the Contact on sms and
// those of its number keep refusing the number, but keyed by its hash (phone.Hash) instead of the
// number. A hashed opt-out of the same scope that already stands (a person erased before with the
// number) makes the number's own redundant, so that one goes. number is "" for a Contact with no
// phone. Run it before the Contact reference is cleared (DetachContact).
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

// DetachContact keeps the Contact's Unsubscribe rows, which are keyed by Destination and keep
// honoring the refusal, and only clears their Contact reference.
func DetachContact(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.Unsubscribe().Update().Where(unsubscribe.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}
