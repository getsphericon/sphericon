package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/throttle"
)

// PurgeAuthAttemptsArgs is the periodic tick that removes stale failed-attempt rows
// (ADR 0025): a row whose last failure is past its window no longer throttles
// anything, and without the purge the table would grow with every address ever tried.
type PurgeAuthAttemptsArgs struct{}

func (PurgeAuthAttemptsArgs) Kind() string { return "auth_attempts_purge" }

type PurgeAuthAttemptsWorker struct {
	river.WorkerDefaults[PurgeAuthAttemptsArgs]

	ent *ent.Client
	now func() time.Time
}

// NewPurgeAuthAttemptsWorker builds the purge of failed sign-in attempts past their window, by the
// Instance clock now.
func NewPurgeAuthAttemptsWorker(client *ent.Client, now func() time.Time) *PurgeAuthAttemptsWorker {
	return &PurgeAuthAttemptsWorker{ent: client, now: now}
}

func (w *PurgeAuthAttemptsWorker) Work(ctx context.Context, _ *river.Job[PurgeAuthAttemptsArgs]) error {
	_, err := throttle.NewAttempts(w.ent, w.now).Purge(ctx)
	return err
}
