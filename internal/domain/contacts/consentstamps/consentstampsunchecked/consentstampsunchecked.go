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

// Module holds the consent stamp step of a Contact's Erasure. It holds nothing: every call takes
// the scope of the Erasure's transaction.
type Module struct{}

// New builds the module.
func New() *Module { return &Module{} }

// EraseInput is what Erase erases: the stamps of Destination on Channel, and the question that
// tells whether an opt-out of a sending source still stands for it.
type EraseInput struct {
	Channel     channels.Channel
	Destination string
	// Standing reports whether the opt-out of the Destination from source still stands.
	Standing func(ctx context.Context, source string) (bool, error)
}

// Erase is the consent stamp part of the Erasure of a Contact that held in.Destination on
// in.Channel, run inside the Erasure's transaction. The stamps follow the opt-outs (ADR 0154):
// where an opt-out of a sending source still stands (in.Standing reports it), the time the person
// stopped keeps ordering the observations that come later, kept by what the stamp is stored by
// (consentstamps.Stored: the number's hash on sms, the Destination elsewhere); otherwise nothing of
// the stamp stays, and neither does the time of a Confirmation, whose proof the Erasure deletes.
func (m *Module) Erase(ctx context.Context, s *ent.Scoped, in EraseInput) error {
	if in.Destination == "" {
		return nil
	}
	stored := consentstamps.Stored(consentstamps.Key{Channel: in.Channel, Destination: in.Destination}).Destination
	rows, err := s.ConsentStamp().Query().Where(
		consentstamp.ChannelEQ(in.Channel), consentstamp.DestinationEQ(stored),
	).All(ctx)
	if err != nil {
		return fmt.Errorf("find consent stamps: %w", err)
	}
	for _, row := range rows {
		stands, err := in.Standing(ctx, row.SendingSource)
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
