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
	"github.com/getsphericon/sphericon/internal/domain/contacts/consentstamps"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Erase is the consent stamp part of the Erasure of a Contact that held destination on channel,
// run inside the Erasure's transaction. The stamps follow the opt-outs (ADR 0154): where an opt-out
// of a sending source still stands (standing reports it), the time the person stopped keeps
// ordering the observations that come later, kept by what the stamp is stored by
// (consentstamps.Stored: the number's hash on sms, the Destination elsewhere); otherwise nothing of
// the stamp stays, and neither does the time of a Confirmation, whose proof the Erasure deletes.
func Erase(ctx context.Context, s *ent.Scoped, channel channels.Channel, destination string,
	standing func(ctx context.Context, source string) (bool, error),
) error {
	if destination == "" {
		return nil
	}
	stored := consentstamps.Stored(consentstamps.Key{Channel: channel, Destination: destination}).Destination
	rows, err := s.ConsentStamp().Query().Where(
		consentstamp.ChannelEQ(channel), consentstamp.DestinationEQ(stored),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("find consent stamps: %w", err)
	}
	for _, row := range rows {
		stands, err := standing(ctx, row.SendingSource)
		if err != nil {
			return err
		}
		if stands && row.OptOutOccurredAt != nil {
			err = s.ConsentStamp().UpdateOneID(row.ID).ClearConfirmationOccurredAt().Exec(ctx)
		} else {
			err = s.ConsentStamp().DeleteOneID(row.ID).Exec(ctx)
		}
		if err != nil {
			return fmt.Errorf("erase consent stamp: %w", err)
		}
	}
	return nil
}
