package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/accounts"
)

// pendingSignupsInterval is how often expired pending sign-ups are pruned: a pending sign-up lives
// ten minutes, so an hour bounds how long an abandoned one's provider data stays.
const pendingSignupsInterval = time.Hour

// PrunePendingSignupsArgs is the hourly tick that deletes the expired pending sign-ups of the
// Sign-in providers (ADR 0062).
type PrunePendingSignupsArgs struct{}

func (PrunePendingSignupsArgs) Kind() string { return "pending_signups_prune" }

// PrunePendingSignupsWorker deletes the provider sign-ups whose Sign-up items were never answered;
// the accounts module decides which are expired.
type PrunePendingSignupsWorker struct {
	river.WorkerDefaults[PrunePendingSignupsArgs]

	accounts *accounts.Accounts
	now      func() time.Time
}

// NewPrunePendingSignupsWorker builds the prune of the pending sign-ups expired by the clock now.
func NewPrunePendingSignupsWorker(acc *accounts.Accounts, now func() time.Time) *PrunePendingSignupsWorker {
	return &PrunePendingSignupsWorker{accounts: acc, now: now}
}

func (w *PrunePendingSignupsWorker) Work(ctx context.Context, _ *river.Job[PrunePendingSignupsArgs]) error {
	deleted, err := w.accounts.PruneExpiredPendingSignups(ctx, w.now())
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired pending sign-ups pruned", "deleted", deleted)
	}
	return nil
}
