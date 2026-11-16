// Package consentstampsunchecked holds the consent stamp part of a Contact's Erasure. It is a step
// of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use case
// internal/usecase/contacterasure runs it in the Erasure's transaction, and only a use case may
// import this one (ADR 0110).
package consentstampsunchecked

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/consentstamp"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Erase is the consent stamp part of the Erasure of a Contact that held destination on channel,
// run inside the Erasure's transaction. The stamps follow the opt-outs (ADR 0154): where an opt-out
// of a sending source still stands (standing reports it), the time the person stopped keeps
// ordering the observations that come later, kept by keptAs as the opt-out is kept (the number's
// hash on sms, the address itself on email); otherwise nothing of the stamp stays, and neither
// does the time of a Confirmation, whose proof the Erasure deletes. A stamp keptAs already holds (a
// person erased before with the number) keeps the later of the two.
func Erase(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination, keptAs string,
	standing func(ctx context.Context, source string) (bool, error),
) error {
	if destination == "" {
		return nil
	}
	rows, err := s.ConsentStamp().Query().Where(
		consentstamp.ChannelEQ(channel), consentstamp.DestinationEQ(destination),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("find consent stamps: %w", err)
	}
	for _, row := range rows {
		stands, err := standing(ctx, row.SendingSource)
		if err != nil {
			return err
		}
		if !stands || row.OptOutOccurredAt == nil {
			err = deleteStamp(ctx, s, row.ID)
		} else {
			err = keep(ctx, s, row, keptAs)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// keep keeps of row only when its stop occurred, by keptAs.
func keep(ctx context.Context, s *ent.Scoped, row *ent.ConsentStamp, keptAs string) error {
	if keptAs == row.Destination {
		err := s.ConsentStamp().UpdateOneID(row.ID).ClearConfirmationOccurredAt().Exec(ctx)
		if err != nil {
			return fmt.Errorf("keep the consent stamp: %w", err)
		}
		return nil
	}
	twin, err := s.ConsentStamp().Query().Where(
		consentstamp.ChannelEQ(row.Channel), consentstamp.DestinationEQ(keptAs), consentstamp.SendingSourceEQ(row.SendingSource),
	).Only(ctx)
	switch {
	case ent.IsNotFound(err):
		err = s.ConsentStamp().UpdateOneID(row.ID).SetDestination(keptAs).ClearConfirmationOccurredAt().
			ClearOptOutIntegrationID().ClearOptOutSequence().Exec(ctx)
		if err != nil {
			return fmt.Errorf("rekey the consent stamp: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("find the kept consent stamp: %w", err)
	}
	if twin.OptOutOccurredAt == nil || row.OptOutOccurredAt.After(*twin.OptOutOccurredAt) {
		err := s.ConsentStamp().UpdateOneID(twin.ID).SetOptOutOccurredAt(*row.OptOutOccurredAt).
			ClearOptOutIntegrationID().ClearOptOutSequence().Exec(ctx)
		if err != nil {
			return fmt.Errorf("keep the later consent stamp: %w", err)
		}
	}
	return deleteStamp(ctx, s, row.ID)
}

func deleteStamp(ctx context.Context, s *ent.Scoped, id int64) error {
	if err := s.ConsentStamp().DeleteOneID(id).Exec(ctx); err != nil {
		return fmt.Errorf("erase consent stamp: %w", err)
	}
	return nil
}
