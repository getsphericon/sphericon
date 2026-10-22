package testhelper

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// PlaceRecipient moves the parked recipient of a Contact (Alice, Bob or Carol, parked on the archived
// Broadcast in the fixtures) onto the Broadcast, with its message if it has one, and returns the
// recipient's id. A test that needs a Broadcast with exactly the recipients it names places them.
func (env *TestEnv) PlaceRecipient(ctx context.Context, broadcastID, contactID int64) int64 {
	s := env.DB.Scoped(fixtures.AcmeID)
	id := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(fixtures.BroadcastArchivedID), broadcastrecipient.ContactID(contactID)).
		OnlyX(ctx).ID
	if err := s.BroadcastRecipient().UpdateOneID(id).SetBroadcastID(broadcastID).Exec(ctx); err != nil {
		panic(err)
	}
	if err := s.OutboundMessage().Update().Where(outboundmessage.BroadcastRecipientID(id)).SetBroadcastID(broadcastID).Exec(ctx); err != nil {
		panic(err)
	}
	return id
}

// ClaimRecipient moves the claim of a placed recipient's message (only Bob's has one) to `age` ago:
// another attempt's claim, live while younger than the send lease.
func (env *TestEnv) ClaimRecipient(ctx context.Context, recipientID int64, age time.Duration) {
	err := env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Update().
		Where(outboundmessage.BroadcastRecipientID(recipientID)).SetClaimedAt(time.Now().Add(-age)).Exec(ctx)
	if err != nil {
		panic(err)
	}
}
