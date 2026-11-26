package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
)

// PruneFrequencyCountsArgs is the daily tick that deletes the count rows of the Frequency cap older
// than the longest period a cap may have (ADR 0178).
type PruneFrequencyCountsArgs struct{}

func (PruneFrequencyCountsArgs) Kind() string { return "frequency_counts_retention" }

// PruneFrequencyCountsWorker deletes the count rows no cap counts any more. It is instance-wide, so
// it holds the raw ent client.
type PruneFrequencyCountsWorker struct {
	river.WorkerDefaults[PruneFrequencyCountsArgs]

	caps *frequencycap.Module
	ent  *ent.Client
}

// NewPruneFrequencyCountsWorker builds the prune of the Frequency cap's count rows of the module.
func NewPruneFrequencyCountsWorker(module *frequencycap.Module, client *ent.Client) *PruneFrequencyCountsWorker {
	return &PruneFrequencyCountsWorker{caps: module, ent: client}
}

func (w *PruneFrequencyCountsWorker) Work(ctx context.Context, _ *river.Job[PruneFrequencyCountsArgs]) error {
	deleted, err := w.caps.Prune(ctx, w.ent, eventsRetentionBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired frequency count rows pruned", "deleted", deleted)
	}
	return nil
}
