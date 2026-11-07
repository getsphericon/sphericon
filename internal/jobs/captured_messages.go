package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
)

// capturedMessagesBatch bounds the rows one DELETE removes so a large backlog never holds a
// long lock.
const capturedMessagesBatch = 500

// capturedMessagesSchedule is when the Captured message purge runs: daily at an off-peak hour, 06:00 UTC.
var capturedMessagesSchedule = mustParseCron("CRON_TZ=UTC 0 6 * * *")

// PruneCapturedMessagesArgs is the daily tick that deletes Captured messages past the Test
// inbox's retention window (ADR 0058).
type PruneCapturedMessagesArgs struct{}

func (PruneCapturedMessagesArgs) Kind() string { return "captured_messages_prune" }

// PruneCapturedMessagesWorker deletes the Captured messages of every Workspace older than the
// retention window; the capturedmessages module does the deleting. It is instance-wide, so it
// holds the raw ent client; a zero retention disables it.
type PruneCapturedMessagesWorker struct {
	river.WorkerDefaults[PruneCapturedMessagesArgs]

	ent       *ent.Client
	retention time.Duration
	now       func() time.Time
}

// NewPruneCapturedMessagesWorker builds the prune of Captured messages older than retention (0 disables it),
// by the Instance clock now.
func NewPruneCapturedMessagesWorker(client *ent.Client, retention time.Duration, now func() time.Time) *PruneCapturedMessagesWorker {
	return &PruneCapturedMessagesWorker{ent: client, retention: retention, now: now}
}

func (w *PruneCapturedMessagesWorker) Work(ctx context.Context, _ *river.Job[PruneCapturedMessagesArgs]) error {
	deleted, err := capturedmessages.Purge(ctx, w.ent, w.retention, capturedMessagesBatch, w.now())
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired captured messages pruned", "deleted", deleted)
	}
	return nil
}
