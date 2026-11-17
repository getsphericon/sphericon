package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
)

// PruneStartTokensArgs is the daily tick that deletes the Start tokens that lapsed or were used
// (ADR 0076).
type PruneStartTokensArgs struct{}

func (PruneStartTokensArgs) Kind() string { return "start_tokens_prune" }

// PruneStartTokensWorker deletes the lapsed and used Start tokens of every Workspace.
type PruneStartTokensWorker struct {
	river.WorkerDefaults[PruneStartTokensArgs]

	ent   *ent.Client
	chats *bindings.Chats
}

// NewPruneStartTokensWorker builds the purge of Start tokens lapsed by the clock of chats.
func NewPruneStartTokensWorker(client *ent.Client, chats *bindings.Chats) *PruneStartTokensWorker {
	return &PruneStartTokensWorker{ent: client, chats: chats}
}

func (w *PruneStartTokensWorker) Work(ctx context.Context, _ *river.Job[PruneStartTokensArgs]) error {
	deleted, err := w.chats.PruneStartTokens(ctx, w.ent)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "lapsed and used start tokens pruned", "deleted", deleted)
	}
	return nil
}
