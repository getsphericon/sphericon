package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// secondFactorReminderInterval is how often the reminder sweep runs: a User is
// reminded within this long of their reminder becoming due.
const secondFactorReminderInterval = 15 * time.Minute

// RemindSecondFactorArgs is the periodic tick that reminds Users whose grace under a
// Two-factor requirement ends within users.ReminderLead (ADR 0020).
type RemindSecondFactorArgs struct{}

func (RemindSecondFactorArgs) Kind() string { return "second_factor_remind" }

// RemindSecondFactorWorker runs the reminder sweep on the Instance clock: each reminder it claims
// is published, and the Notices' consumer mails it (ADR 0105).
type RemindSecondFactorWorker struct {
	river.WorkerDefaults[RemindSecondFactorArgs]

	ent *ent.Client
	bus *events.Bus
	now func() time.Time
}

// NewRemindSecondFactorWorker builds the reminder sweep on the Instance clock now.
func NewRemindSecondFactorWorker(client *ent.Client, bus *events.Bus, now func() time.Time) *RemindSecondFactorWorker {
	return &RemindSecondFactorWorker{ent: client, bus: bus, now: now}
}

func (w *RemindSecondFactorWorker) Work(ctx context.Context, _ *river.Job[RemindSecondFactorArgs]) error {
	return users.RemindSecondFactorDeadlines(ctx, w.ent, w.bus, w.now())
}
