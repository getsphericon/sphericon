package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// The unsubscribe of a recipient keeps the earliest accepted observation, by a conditional
// update, and reports whether the stamp changed. Repeated facts still count the recipient once.
// Its opens and clicks
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
	n, err := s.BroadcastRecipient().Update().Where(
		broadcastrecipient.ID(*msg.BroadcastRecipientID), broadcastrecipient.BroadcastID(*msg.BroadcastID),
		broadcastrecipient.Or(broadcastrecipient.UnsubscribedAtIsNil(), broadcastrecipient.UnsubscribedAtGT(at)),
	).SetUnsubscribedAt(at).Save(ctx)
	return n > 0, err
}
