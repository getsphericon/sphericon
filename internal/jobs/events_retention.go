package jobs

import (
	"context"

	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// eventsRetentionBatch bounds the rows one DELETE removes so a large backlog
// never holds a long lock.
const eventsRetentionBatch = 2000

// eventsRetentionSchedule is when the Event retention job runs: daily at an off-peak hour, 03:00 UTC.
var eventsRetentionSchedule = mustParseCron("CRON_TZ=UTC 0 3 * * *")

// PruneEventsArgs is the daily tick that deletes expired analytical Events
// (ADR 0019).
type PruneEventsArgs struct{}

func (PruneEventsArgs) Kind() string { return "events_retention" }

// PruneEventsWorker deletes analytical Events older than the retention window.
// Evidentiary Events are never deleted by age. It is instance-wide, so it holds
// the raw ent client; a zero retention disables it.
type PruneEventsWorker struct {
	river.WorkerDefaults[PruneEventsArgs]

	ent       *ent.Client
	retention time.Duration
}

// NewPruneEventsWorker builds the prune of analytical Events older than retention (0 disables it).
func NewPruneEventsWorker(client *ent.Client, retention time.Duration) *PruneEventsWorker {
	return &PruneEventsWorker{ent: client, retention: retention}
}

func (w *PruneEventsWorker) Work(ctx context.Context, _ *river.Job[PruneEventsArgs]) error {
	deleted, err := events.PruneEvents(ctx, w.ent, w.retention, eventsRetentionBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired events pruned", "deleted", deleted)
	}
	firings, err := rules.PruneFirings(ctx, w.ent, w.retention, eventsRetentionBatch)
	if err != nil {
		return err
	}
	if firings > 0 {
		slog.InfoContext(ctx, "expired rule firings pruned", "deleted", firings)
	}
	return nil
}
