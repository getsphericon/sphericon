package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/infra/logging"
)

// startTokensHourUTC is the off-peak hour the purge of Start tokens runs at.
const startTokensHourUTC = 4

// PruneStartTokensArgs is the daily tick that deletes the Start tokens that lapsed or were used
// (ADR 0076).
type PruneStartTokensArgs struct{}

func (PruneStartTokensArgs) Kind() string { return "start_tokens_prune" }

// PruneStartTokensWorker deletes the lapsed and used Start tokens of every Workspace.
type PruneStartTokensWorker struct {
	river.WorkerDefaults[PruneStartTokensArgs]

	ent *ent.Client
	now func() time.Time
}

// NewPruneStartTokensWorker builds the purge of Start tokens lapsed by the clock now.
func NewPruneStartTokensWorker(client *ent.Client, now func() time.Time) *PruneStartTokensWorker {
	return &PruneStartTokensWorker{ent: client, now: now}
}

func (w *PruneStartTokensWorker) Work(ctx context.Context, _ *river.Job[PruneStartTokensArgs]) error {
	deleted, err := bindings.PruneStartTokens(ctx, w.ent, w.now())
	if err != nil {
		return err
	}
	if deleted > 0 {
		logging.FromContext(ctx).Info("lapsed and used start tokens pruned", "deleted", deleted)
	}
	return nil
}
