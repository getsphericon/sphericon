package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
)

// --- the release of a deleted Telegram bot's webhook (ADR 0145) ---

type ReleaseWebhookWorker struct {
	river.WorkerDefaults[integrations.ReleaseWebhookArgs]

	ent          *ent.Client
	integrations *integrations.Module
}

// NewReleaseWebhookWorker builds the worker that lets a deleted Telegram bot go of its webhook
// through its module.
func NewReleaseWebhookWorker(client *ent.Client, mod *integrations.Module) *ReleaseWebhookWorker {
	return &ReleaseWebhookWorker{ent: client, integrations: mod}
}

// Work releases the webhook of the Integration the deletion queued the job for
// (integrations.Module.ReleaseWebhook), best effort. An Integration gone since, with its Workspace's
// erase, is nothing to do.
func (w *ReleaseWebhookWorker) Work(ctx context.Context, job *river.Job[integrations.ReleaseWebhookArgs]) error {
	row, err := w.ent.Integration.Get(ctx, job.Args.IntegrationID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	w.integrations.ReleaseWebhook(ctx, row)
	return nil
}
