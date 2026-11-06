package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/usecase/ingesthook"
)

// LateNoticeWorker reads again a Delivery notice that matched no message when it reached the
// Ingest hook (ingest.LateNoticeArgs).
type LateNoticeWorker struct {
	river.WorkerDefaults[ingesthook.LateNoticeArgs]

	ent    *ent.Client
	ingest *ingesthook.UseCase
	now    func() time.Time
}

// NewLateNoticeWorker builds the worker over the raw client the job entry point scopes from. A
// Delivery notice needs no Telegram bot, so the module reads it without one.
func NewLateNoticeWorker(client *ent.Client, bus *events.Bus, now func() time.Time) *LateNoticeWorker {
	return &LateNoticeWorker{ent: client, ingest: ingesthook.New(bus, nil), now: now}
}

func (w *LateNoticeWorker) Work(ctx context.Context, job *river.Job[ingesthook.LateNoticeArgs]) error {
	// Job entry point (ADR 0017): the scope is built from the loaded Integration's Workspace, as
	// the Ingest hook builds it. An Integration deleted since ends the job quietly.
	row, err := w.ent.Integration.Get(ctx, job.Args.IntegrationID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.ingest.InterpretLate(ctx, events.Ingest(w.ent.Scoped(row.WorkspaceID)), row, job.Args, w.now())
}
