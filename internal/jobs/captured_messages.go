package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domain/sending/capturedmessages"
)

// capturedMessagesBatch bounds the rows one DELETE removes so a large backlog never holds a
// long lock.
const capturedMessagesBatch = 500

// PruneCapturedMessagesArgs is the daily tick that deletes Captured messages past the Test
// inbox's retention window (ADR 0058).
type PruneCapturedMessagesArgs struct{}

func (PruneCapturedMessagesArgs) Kind() string { return "captured_messages_prune" }

// PruneCapturedMessagesWorker deletes the Captured messages of every Workspace older than the
// retention window; the capturedmessages module does the deleting, Instance-wide, by its clock. A
// zero retention disables it.
type PruneCapturedMessagesWorker struct {
	river.WorkerDefaults[PruneCapturedMessagesArgs]

	captured  *capturedmessages.Module
	retention time.Duration
}

// NewPruneCapturedMessagesWorker builds the prune of Captured messages older than retention (0
// disables it).
func NewPruneCapturedMessagesWorker(captured *capturedmessages.Module, retention time.Duration) *PruneCapturedMessagesWorker {
	return &PruneCapturedMessagesWorker{captured: captured, retention: retention}
}

func (w *PruneCapturedMessagesWorker) Work(ctx context.Context, _ *river.Job[PruneCapturedMessagesArgs]) error {
	deleted, err := w.captured.Purge(ctx, w.retention, capturedMessagesBatch)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.InfoContext(ctx, "expired captured messages pruned", "deleted", deleted)
	}
	return nil
}
