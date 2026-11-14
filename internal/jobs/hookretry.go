package jobs

import (
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
)

// hookRetries is the WorkerDefaults of a job that calls out over HTTP (a Webhook delivery, an
// Automation's HTTP step, a Rule's http Action): its retries follow webhook.HookNextRetry, days
// rather than river's default hours, spread by jitter. Its args insert with the matching
// webhook.HookMaxAttempts.
type hookRetries[T river.JobArgs] struct {
	river.WorkerDefaults[T]

	jitter webhook.HookJitter
}

func (h hookRetries[T]) NextRetry(job *river.Job[T]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt, h.jitter)
}

// hookAttempt is the attempt of job as the HTTP call's retry decision reads it.
func hookAttempt(job *rivertype.JobRow) httpcall.Attempt {
	return httpcall.Attempt{Number: job.Attempt, Limit: job.MaxAttempts, CreatedAt: job.CreatedAt}
}
