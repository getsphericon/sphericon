package consent

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// The Confirmation and Unsubscribe parts of a Contact's Erasure (ADR 0021). Each runs inside the
// Erasure's transaction (s comes from it).

// EraseConfirmations deletes the Contact's Confirmation and its evidence; losing it fails safe
// (re-confirmation). On sms the Confirmation of the Contact's phone goes too when it names no
// Contact: it was recorded before any Contact had the number (by the API or a START). phone is ""
// for a Contact with no phone.
func EraseConfirmations(ctx context.Context, s *ent.Scoped, contactID int64, phone string) error {
	match := confirmation.ContactID(contactID)
	if phone != "" {
		match = confirmation.Or(match, confirmation.And(
			confirmation.ChannelEQ(channels.SMS), confirmation.DestinationEQ(phone)))
	}
	_, err := s.Confirmation().Delete().Where(match).Exec(ctx)
	return err
}

// DetachUnsubscribes keeps the Contact's Unsubscribe rows, which are keyed by Destination and keep
// honoring the refusal, and only clears their Contact reference.
func DetachUnsubscribes(ctx context.Context, s *ent.Scoped, contactID int64) error {
	_, err := s.Unsubscribe().Update().Where(unsubscribe.ContactID(contactID)).ClearContactID().Save(ctx)
	return err
}
