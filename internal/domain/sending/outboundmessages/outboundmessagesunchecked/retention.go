package outboundmessagesunchecked

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// The deletes of message retention (ADR 0143), which the use case internal/usecase/messageretention
// runs once the figures the messages made are kept: alone, each would take a report's figures with
// the rows. Each message's tracked links go with it (ON DELETE CASCADE).

// DeleteOfRecipients deletes the messages of the Broadcast recipients, in the transaction that
// deletes the recipients, so none of them reads as pending without its message.
func DeleteOfRecipients(ctx context.Context, s *ent.Scoped, recipientIDs []int64) (int, error) {
	return s.OutboundMessage().Delete().Where(outboundmessage.BroadcastRecipientIDIn(recipientIDs...)).Exec(ctx)
}

// DeleteOfBroadcast deletes up to limit of the Broadcast's messages and returns how many: the ones
// left once its recipients are gone, whose recipient an Erasure had removed.
func DeleteOfBroadcast(ctx context.Context, s *ent.Scoped, broadcastID int64, limit int) (int, error) {
	ids, err := s.OutboundMessage().Query().
		Where(outboundmessage.BroadcastID(broadcastID)).
		Limit(limit).
		IDs(ctx)
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	return s.OutboundMessage().Delete().Where(outboundmessage.IDIn(ids...)).Exec(ctx)
}

// TakeExpired deletes up to limit of the Workspace's messages of the kind created before cutoff
// that have left pending, oldest first, and returns them as they were, so the caller counts in the
// same transaction what they made. The rows are locked until the commit, so a late stamp waits and
// finds its message gone rather than slipping past the count.
func TakeExpired(ctx context.Context, s *ent.Scoped, kind outboundmessage.Kind, cutoff time.Time, limit int) ([]*ent.OutboundMessage, error) {
	msgs, err := s.OutboundMessage().Query().
		Where(
			outboundmessage.KindEQ(kind),
			outboundmessage.CreatedAtLT(cutoff),
			outboundmessage.StatusNEQ(outboundmessage.StatusPending),
		).
		Order(outboundmessage.ByCreatedAt()).
		Limit(limit).
		ForUpdate().
		All(ctx)
	if err != nil || len(msgs) == 0 {
		return nil, err
	}
	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	if _, err := s.OutboundMessage().Delete().Where(outboundmessage.IDIn(ids...)).Exec(ctx); err != nil {
		return nil, err
	}
	return msgs, nil
}
