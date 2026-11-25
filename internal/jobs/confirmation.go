package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
)

// SendConfirmationWorker sends the email of one Confirmation request (ADR 0013).
type SendConfirmationWorker struct {
	river.WorkerDefaults[confirmationrequests.SendArgs]

	ent      *ent.Client
	requests *confirmationrequests.Module
}

// NewSendConfirmationWorker builds the worker that sends a Confirmation request's email through
// requests.
func NewSendConfirmationWorker(client *ent.Client, requests *confirmationrequests.Module) *SendConfirmationWorker {
	return &SendConfirmationWorker{ent: client, requests: requests}
}

// Work is the river adapter over the module's delivery: a wait is a snooze, which spends no attempt,
// and any other error spends one.
func (w *SendConfirmationWorker) Work(ctx context.Context, job *river.Job[confirmationrequests.SendArgs]) error {
	attempt := confirmationrequests.Attempt{Number: job.Attempt, Limit: job.MaxAttempts}
	out := w.requests.Deliver(ctx, w.ent.Scoped(job.Args.WorkspaceID), job.Args, attempt)
	if out.Wait > 0 {
		return river.JobSnooze(out.Wait)
	}
	return out.Err
}
