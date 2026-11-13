package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/callbackinterpretation"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// LateNoticeWorker reads again a Delivery notice that matched no message when it reached the
// Ingest hook (callbackinterpretation.LateNoticeArgs).
type LateNoticeWorker struct {
	river.WorkerDefaults[callbackinterpretation.LateNoticeArgs]

	ent    *ent.Client
	ingest *callbackinterpretation.Module
}

// NewLateNoticeWorker builds the worker over the raw client the job entry point scopes from and
// the Callback interpretation the container built (ADR 0111).
func NewLateNoticeWorker(client *ent.Client, ingest *callbackinterpretation.Module) *LateNoticeWorker {
	return &LateNoticeWorker{ent: client, ingest: ingest}
}

func (w *LateNoticeWorker) Work(ctx context.Context, job *river.Job[callbackinterpretation.LateNoticeArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Integration's Workspace, as
	// the Ingest hook builds it. An Integration deleted since ends the job quietly.
	row, err := w.ent.Integration.Get(ctx, job.Args.IntegrationID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.ingest.InterpretLate(ctx, events.Ingest(w.ent.Scoped(row.WorkspaceID)), row, job.Args)
}
