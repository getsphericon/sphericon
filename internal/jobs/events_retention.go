package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/events"
)

// eventsRetentionBatch bounds the rows one DELETE removes so a large backlog
// never holds a long lock.
const eventsRetentionBatch = 2000

// eventsRetentionHourUTC is the off-peak hour the Event retention job runs at.
const eventsRetentionHourUTC = 3

// dailyAtUTC is a river periodic schedule firing once a day at a fixed UTC hour:
// an off-peak run, which river's interval schedule cannot express.
type dailyAtUTC struct{ hour int }

func (s dailyAtUTC) Next(t time.Time) time.Time {
	t = t.UTC()
	next := time.Date(t.Year(), t.Month(), t.Day(), s.hour, 0, 0, 0, time.UTC)
	if !next.After(t) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// PruneEventsArgs is the daily tick that deletes expired analytical Events
// (ADR 0019).
type PruneEventsArgs struct{}

func (PruneEventsArgs) Kind() string { return "events_retention" }

// PruneEventsWorker deletes analytical Events older than the retention window.
// Evidentiary Events are never deleted by age. It is instance-wide, so it holds
// the raw database handle; a zero retention disables it.
type PruneEventsWorker struct {
	river.WorkerDefaults[PruneEventsArgs]
	db        *sql.DB
	retention time.Duration
}

func (w *PruneEventsWorker) Work(ctx context.Context, _ *river.Job[PruneEventsArgs]) error {
	deleted, err := events.PruneEvents(ctx, w.db, w.retention, eventsRetentionBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired events pruned", "deleted", deleted)
	}
	return nil
}
