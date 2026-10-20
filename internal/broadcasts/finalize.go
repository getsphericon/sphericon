package broadcasts

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// Finalize flips a Broadcast to "sent" once none of its recipients are still
// pending, deriving the aggregate counters from the recipient rows. The
// finalize move (sending -> sent, a compare-and-swap) makes concurrent finalizers (one per
// per-recipient job, or an Erasure that removed the last pending recipient) a no-op
// after the first, as is a Broadcast that is not sending at all, so sent_at is set exactly once and the counters self-heal
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

	err := fire(ctx, s, broadcastID, triggerFinalize, func(u *ent.BroadcastScopedUpdate) {
		u.SetSentAt(time.Now()).
			SetSentCount(counts[broadcastrecipient.StatusSent]).
			SetFailedCount(counts[broadcastrecipient.StatusFailed]).
			SetSkippedCount(counts[broadcastrecipient.StatusSkipped]).
			ClearHoldReason()
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) || errors.Is(err, ErrNotFound) {
		return nil // not sending (or gone): another finalizer won, or there is nothing to settle
	}
	return err
}
