// Package unsubscribesunchecked holds the SMS opt-out part of a Contact's Erasure. It is a step of
// a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use case
// internal/usecase/contacterasure runs each owner's step in its order, in the one transaction of
// the Erasure; only a use case may import this one (ADR 0110). The unsubscribes module does not
// import it.
package unsubscribesunchecked

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// EraseSMS is the SMS opt-out part of a Contact's Erasure (ADR 0021), run inside the Erasure's
// transaction (s comes from it): the opt-outs of the Contact on sms and
// those of its number keep refusing the number, but keyed by its hash (phone.Hash) instead of the
// number. A hashed opt-out of the same scope that already stands (a person erased before with the
// number) makes the number's own redundant, so that one goes. number is "" for a Contact with no
// phone. Run it before the Contact row goes, whose foreign key clears the opt-outs' Contact
// reference.
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
