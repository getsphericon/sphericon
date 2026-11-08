// Package confirmationsunchecked holds the Confirmation part of a Contact's Erasure that the
// foreign keys cannot do. It is a step of a Contact's Erasure (ADR 0021), which harms a live
// Contact when it runs alone: the use case internal/usecase/contacterasure runs each owner's step
// in its order, in the one transaction of the Erasure; only a use case may import this one (ADR
// 0110). The confirmations module does not import it.
package confirmationsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// EraseNumber is the Confirmation part of a Contact's Erasure (ADR 0021) that the foreign keys
// cannot do, run inside the Erasure's transaction (s comes from it): the sms Confirmation of the
// Contact's phone names no Contact when it was recorded before any Contact had the number (by the
// API or a START), so it does not go with the Contact row, whose own Confirmations cascade.
// Losing it fails safe (re-confirmation). phone is "" for a Contact with no phone.
func EraseNumber(ctx context.Context, s *ent.Scoped, phone string) error {
	if phone == "" {
		return nil
	}
	_, err := s.Confirmation().Delete().
		Where(confirmation.ChannelEQ(channels.SMS), confirmation.DestinationEQ(phone)).
		Exec(ctx)
	return err
}
