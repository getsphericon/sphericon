package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
)

// secondFactorReminderInterval is how often the reminder sweep runs: a User is
// reminded within this long of their reminder becoming due.
const secondFactorReminderInterval = 15 * time.Minute

// RemindSecondFactorArgs is the periodic tick that reminds Users whose grace under a
// Two-factor requirement ends within secondfactors.ReminderLead (ADR 0020).
type RemindSecondFactorArgs struct{}

func (RemindSecondFactorArgs) Kind() string { return "second_factor_remind" }

// RemindSecondFactorWorker runs the reminder sweep on the Instance clock: each reminder it claims
// is published, and the Notices' consumer mails it (ADR 0105).
type RemindSecondFactorWorker struct {
	river.WorkerDefaults[RemindSecondFactorArgs]

	factors *secondfactors.Module
}

// NewRemindSecondFactorWorker builds the reminder sweep of the Second factors module, on its clock.
func NewRemindSecondFactorWorker(factors *secondfactors.Module) *RemindSecondFactorWorker {
	return &RemindSecondFactorWorker{factors: factors}
}

func (w *RemindSecondFactorWorker) Work(ctx context.Context, _ *river.Job[RemindSecondFactorArgs]) error {
	return w.factors.RemindDeadlines(ctx)
}
