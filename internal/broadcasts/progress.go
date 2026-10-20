package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
)

// Progress is how far a sending Broadcast has got (ADR 0023). A Broadcast stays
// "sending" while it is paced, held or deferred, so the progress is derived from its
// recipients, not from a status.
type Progress struct {
	// Processed counts recipients already decided: sent, skipped or failed.
	Processed int
	// Remaining counts recipients still pending.
	Remaining int
	// EstimatedCompletion is when the last remaining recipient is expected to go out;
	// nil when it cannot be known (a hold, or no Send rate limit to pace by).
	EstimatedCompletion *time.Time
}

// ProgressOf computes the progress of a Broadcast, or nil when it is not sending.
// The estimate is the last scheduled recipient job while that is still ahead; once it
// has passed (jobs were deferred, or the limit was lowered mid-send) it falls back to
// the remaining recipients at the Integration's current rate. A hold has no estimate:
// the source is blocked, not busy, and nothing says when it lifts.
func ProgressOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, now time.Time) (*Progress, error) {
	if b.Status != broadcast.StatusSending {
		return nil, nil
	}
	pending, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.BroadcastID(b.ID), broadcastrecipient.StatusEQ(broadcastrecipient.StatusPending)).
		Count(ctx)
	if err != nil {
		return nil, err
	}
	p := &Progress{Remaining: pending, Processed: max(0, b.RecipientsTotal-pending)}
	if pending == 0 || b.HoldReason != nil {
		return p, nil
	}
	if b.LastScheduledAt != nil && b.LastScheduledAt.After(now) {
		p.EstimatedCompletion = b.LastScheduledAt
		return p, nil
	}
	step, err := interval(ctx, s)
	if err != nil {
		return nil, err
	}
	if step > 0 {
		eta := now.Add(time.Duration(pending) * step)
		p.EstimatedCompletion = &eta
	}
	return p, nil
}
