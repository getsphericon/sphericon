// Package suppressionsunchecked holds the Suppression part of a Contact's Erasure. It is a step of
// a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use case
// internal/usecase/contacterasure runs each owner's step in its order, in the one transaction of
// the Erasure; only a use case may import this one (ADR 0110). The suppressions module does not
// import it.
package suppressionsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// EraseContact is the Suppression part of the Contact's Erasure: its lifted rows are
// deleted, since a lifted one refuses nothing any more. Its active rows stay, because they are
// keyed by destination and keep refusing the address; they lose the Contact reference when the
// Contact row goes, whose foreign key sets it null. On sms a number leaves only its hash
// (ADR 0021): the rows of the Contact's phone, number ("" when it has none), count as its own,
// and an active one keeps refusing by phone.Hash of the number. A hashed one already standing (a
// person erased before with the number) makes it redundant.
func EraseContact(ctx context.Context, s *ent.Scoped, contactID int64, number string) error {
	mine := entsuppression.ContactID(contactID)
	if number != "" {
		mine = entsuppression.Or(mine, entsuppression.And(
			entsuppression.ChannelEQ(channels.SMS), entsuppression.DestinationEQ(number)))
	}
	if _, err := s.Suppression().Delete().Where(mine, entsuppression.LiftedAtNotNil()).Exec(ctx); err != nil {
		return err
	}
	return hashNumbers(ctx, s, mine)
}

// hashNumbers rekeys the active sms Suppressions mine matches by the hash of their number.
func hashNumbers(ctx context.Context, s *ent.Scoped, mine predicate.Suppression) error {
	rows, err := s.Suppression().Query().
		Where(mine, entsuppression.ChannelEQ(channels.SMS), entsuppression.LiftedAtIsNil()).
		All(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		hashed := phone.Hash(r.Destination)
		twin, err := s.Suppression().Query().Where(
			entsuppression.ChannelEQ(channels.SMS),
			entsuppression.DestinationEQ(hashed),
			entsuppression.LiftedAtIsNil(),
		).Exist(ctx)
		if err != nil {
			return err
		}
		if twin {
			err = s.Suppression().DeleteOneID(r.ID).Exec(ctx)
		} else {
			err = s.Suppression().UpdateOneID(r.ID).SetDestination(hashed).Exec(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
