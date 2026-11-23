package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
)

// PruneSkippedEntriesArgs is the daily tick that deletes the skipped entries into Automations past
// the analytical Events' window (ADR 0019, ADR 0127).
type PruneSkippedEntriesArgs struct{}

func (PruneSkippedEntriesArgs) Kind() string { return "automation_skipped_entries_retention" }

// PruneSkippedEntriesWorker deletes the skipped entries older than the Event retention window. It is
// instance-wide, so it holds the raw ent client; a zero retention disables it.
type PruneSkippedEntriesWorker struct {
	river.WorkerDefaults[PruneSkippedEntriesArgs]

	automations *automations.Module
	ent         *ent.Client
	retention   time.Duration
}

// NewPruneSkippedEntriesWorker builds the prune of the skipped entries into the Automations of the
// module, older than retention (0 disables it).
func NewPruneSkippedEntriesWorker(module *automations.Module, client *ent.Client, retention time.Duration) *PruneSkippedEntriesWorker {
	return &PruneSkippedEntriesWorker{automations: module, ent: client, retention: retention}
}

func (w *PruneSkippedEntriesWorker) Work(ctx context.Context, _ *river.Job[PruneSkippedEntriesArgs]) error {
	deleted, err := w.automations.PruneSkippedEntries(ctx, w.ent, w.retention, eventsRetentionBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired automation skipped entries pruned", "deleted", deleted)
	}
	return nil
}
