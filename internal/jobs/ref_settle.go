package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
)

const (
	refSettleInterval = time.Hour
	refSettleBatch    = 1000
)

// ClearContactRefsArgs is the periodic tick that settles the rows whose contact_id points at a
// Contact that is gone, then validates the keys the migration added NOT VALID (ADR 0033).
type ClearContactRefsArgs struct{}

func (ClearContactRefsArgs) Kind() string { return "clear_contact_refs" }

// ClearOutboundMessageRefsArgs is the periodic tick that clears the ids on Outbound messages that
// point at a row which is gone, then validates the keys the migration added NOT VALID (ADR 0033).
type ClearOutboundMessageRefsArgs struct{}

func (ClearOutboundMessageRefsArgs) Kind() string { return "clear_outbound_message_refs" }

// refSettleWorker runs one settle step of internal/refsettle, which owns the work.
type refSettleWorker[A river.JobArgs] struct {
	river.WorkerDefaults[A]

	settle  func(ctx context.Context, batch int) (int, error)
	message string
}

func (w *refSettleWorker[A]) Work(ctx context.Context, _ *river.Job[A]) error {
	settled, err := w.settle(ctx, refSettleBatch)
	if settled > 0 {
		slog.InfoContext(ctx, w.message, "rows", settled)
	}
	return err
}

// refSettlePeriodic ticks a refSettleWorker's args hourly and once at start.
func refSettlePeriodic[A river.JobArgs](args A) *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(refSettleInterval),
		func() (river.JobArgs, *river.InsertOpts) { return args, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}
