package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/accounts"
)

// unverifiedUsersBatch bounds the rows one DELETE removes so a large backlog never
// holds a long lock.
const unverifiedUsersBatch = 500

// unverifiedUsersHourUTC is the off-peak hour the Unverified User prune runs at.
const unverifiedUsersHourUTC = 4

// PruneUnverifiedUsersArgs is the daily tick that deletes stale Unverified Users
// (ADR 0030).
type PruneUnverifiedUsersArgs struct{}

func (PruneUnverifiedUsersArgs) Kind() string { return "unverified_users_prune" }

// PruneUnverifiedUsersWorker frees the addresses held by Sign-ups whose verify
// link was never used; the accounts module decides which Users are stale. A zero
// retention disables it.
type PruneUnverifiedUsersWorker struct {
	river.WorkerDefaults[PruneUnverifiedUsersArgs]

	accounts  *accounts.Accounts
	retention time.Duration
}

func (w *PruneUnverifiedUsersWorker) Work(ctx context.Context, _ *river.Job[PruneUnverifiedUsersArgs]) error {
	deleted, err := w.accounts.PruneUnverified(ctx, w.retention, unverifiedUsersBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "stale unverified users pruned", "deleted", deleted)
	}
	return nil
}
