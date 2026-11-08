package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// The unsubscribe of a recipient is set once, by a conditional update, and reported as whether
// this call set it, so a repeated click on the link is counted exactly once. Its opens and clicks
// are the Outbound message's stamps (outboundmessages.StampOpened, ADR 0053).

// RecordUnsubscribe attributes an opt-out to a received message. Its anonymous delivery and
// recipient references survive Erasure; a missing, foreign or unsent message counts nothing.
func RecordUnsubscribe(ctx context.Context, s *ent.Scoped, messageID int64, at time.Time) (bool, error) {
	msg, err := s.OutboundMessage().Query().Where(outboundmessage.ID(messageID), outboundmessage.StatusEQ(outboundmessage.StatusSent)).Only(ctx)
	if ent.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if msg.BroadcastRecipientID == nil || msg.BroadcastID == nil {
		return false, nil
	}
	return setOnce(ctx, s,
		broadcastrecipient.And(broadcastrecipient.ID(*msg.BroadcastRecipientID), broadcastrecipient.BroadcastID(*msg.BroadcastID)),
		broadcastrecipient.UnsubscribedAtIsNil(),
		func(u *ent.BroadcastRecipientScopedUpdate, at time.Time) { u.SetUnsubscribedAt(at) }, at)
}

func setOnce(ctx context.Context, s *ent.Scoped, which, unset predicate.BroadcastRecipient, set func(*ent.BroadcastRecipientScopedUpdate, time.Time), at time.Time) (bool, error) {
	u := s.BroadcastRecipient().Update().Where(which, unset)
	set(u, at)
	n, err := u.Save(ctx)
	return n > 0, err
}
