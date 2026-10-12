package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
)

// Finalize flips a Broadcast to "sent" once none of its recipients are still
// pending, deriving the aggregate counters from the recipient rows. The
// conditional WHERE status=sending makes concurrent finalizers (one per
// per-recipient job, or an Erasure that removed the last pending recipient) a no-op
// after the first, so sent_at is set exactly once and the counters self-heal
// against any retry drift.
func Finalize(ctx context.Context, s *ent.Scoped, broadcastID int64) error {
	var rows []struct {
		Status broadcastrecipient.Status `json:"status"`
		Count  int                       `json:"count"`
	}
	if err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(broadcastID)).
		GroupBy(broadcastrecipient.FieldStatus).
		Aggregate(ent.Count()).
		Scan(ctx, &rows); err != nil {
		return err
	}
	counts := map[broadcastrecipient.Status]int{}
	for _, r := range rows {
		counts[r.Status] = r.Count
	}
	if counts[broadcastrecipient.StatusPending] > 0 {
		return nil // not all recipients resolved yet
	}

	_, err := s.Broadcast().Update().
		Where(broadcast.IDEQ(broadcastID), broadcast.StatusEQ(broadcast.StatusSending)).
		SetStatus(broadcast.StatusSent).
		SetSentAt(time.Now()).
		SetSentCount(counts[broadcastrecipient.StatusSent]).
		SetFailedCount(counts[broadcastrecipient.StatusFailed]).
		SetSkippedCount(counts[broadcastrecipient.StatusSkipped]).
		ClearHoldReason().
		Save(ctx)
	return err
}
