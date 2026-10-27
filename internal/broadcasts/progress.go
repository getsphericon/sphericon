package broadcasts

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
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

// Detail is one Broadcast's figures and, while it is sending, its Progress, both from a
// single count of its recipients. A list of Broadcasts reads StatsOf instead: no Progress,
// two grouped queries for the page (ADR 0023).
type Detail struct {
	Stats    Stats
	Progress *Progress
}

// DetailOf reads a Broadcast's figures and, when it is sending, its Progress.
func DetailOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, now time.Time) (Detail, error) {
	figures, err := StatsOf(ctx, s, b.ID)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Stats: figures[b.ID]}
	if b.Status != broadcast.StatusSending {
		return d, nil
	}
	d.Progress, err = progressOf(ctx, s, b, d.Stats, now)
	return d, err
}

// progressOf computes the progress of a sending Broadcast from its figures.
// The estimate is the last scheduled recipient job while that is still ahead; once it
// has passed (jobs were deferred, or the limit was lowered mid-send) it falls back to
// the remaining recipients at the Integration's current rate. A hold has no estimate:
// the source is blocked, not busy, and nothing says when it lifts.
func progressOf(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, st Stats, now time.Time) (*Progress, error) {
	pending := st.Pending
	p := &Progress{Remaining: pending, Processed: st.Processed()}
	if pending == 0 || b.HoldReason != nil {
		return p, nil
	}
	if b.LastScheduledAt != nil && b.LastScheduledAt.After(now) {
		p.EstimatedCompletion = b.LastScheduledAt
		return p, nil
	}
	step, err := interval(ctx, s, b)
	if err != nil {
		return nil, err
	}
	if step > 0 {
		eta := now.Add(time.Duration(pending) * step)
		p.EstimatedCompletion = &eta
	}
	return p, nil
}
