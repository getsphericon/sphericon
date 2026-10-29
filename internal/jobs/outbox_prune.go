package jobs

import (
	"context"

	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
)

// outboxPruneInterval is how often the outbox is pruned; outboxPruneBatch bounds
// the rows one DELETE removes so a large backlog never holds a long lock.
const (
	outboxPruneInterval = 10 * time.Minute
	outboxPruneBatch    = 5000
)

// PruneOutboxArgs is the periodic tick that prunes the domain-events outbox
// (ADR 0019).
type PruneOutboxArgs struct{}

func (PruneOutboxArgs) Kind() string { return "outbox_prune" }

// PruneOutboxWorker deletes outbox rows every registered consumer group has
// consumed and that are older than the floor. It is instance-wide (the outbox is
// not Workspace-scoped), so it holds the raw ent client.
type PruneOutboxWorker struct {
	river.WorkerDefaults[PruneOutboxArgs]

	ent   *ent.Client
	floor time.Duration
}

// NewPruneOutboxWorker builds the outbox prune, which never deletes a row younger than floor.
func NewPruneOutboxWorker(client *ent.Client, floor time.Duration) *PruneOutboxWorker {
	return &PruneOutboxWorker{ent: client, floor: floor}
}

func (w *PruneOutboxWorker) Work(ctx context.Context, _ *river.Job[PruneOutboxArgs]) error {
	res, err := events.PruneOutbox(ctx, w.ent, w.floor, outboxPruneBatch)
	if err != nil {
		return err
	}
	switch {
	case res.Skipped:
		slog.WarnContext(ctx, "outbox prune skipped: a registered consumer group has no cursor yet")
	case res.Deleted > 0 || res.RetiredGroups > 0:
		slog.InfoContext(ctx, "outbox pruned", "deleted", res.Deleted, "retired_groups", res.RetiredGroups)
	}
	return nil
}
