package automations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// The Enrollment's lifecycle (ADR 0046): it starts `active` and leaves it exactly once, for
// `completed`, `failed` or `exited`; all three are final. Every move is a compare-and-swap
// from `active`, so whichever writer reaches the row first decides a race, and a late step
// cannot overwrite an exit (the move reports lifecycle.ErrInvalidTransition instead).
const (
	triggerComplete = "complete"
	triggerFail     = "fail"
	triggerExit     = "exit"
)

// Failure is what an Enrollment keeps of the step that failed it. The zero value records
// nothing beyond the `failed` status.
type Failure struct {
	Kind    automationrun.FailureKind
	Status  int
	Excerpt string
}

// move fires trigger on one Enrollment, making the move out of `active` a conditional
// update. extra adds fields to the same write.
func move(ctx context.Context, s *ent.Scoped, runID int64, trigger string, extra func(*ent.AutomationRunScopedUpdate)) error {
	m := lifecycle.New(lifecycle.Storage[automationrun.Status]{
		Read: func(ctx context.Context) (automationrun.Status, error) {
			run, err := s.AutomationRun().Get(ctx, runID)
			if err != nil {
				return "", err
			}
			return run.Status, nil
		},
		Write: func(ctx context.Context, from, to automationrun.Status) (bool, error) {
			u := s.AutomationRun().Update().
				Where(automationrun.ID(runID), automationrun.StatusEQ(from)).
				SetStatus(to).
				ClearResumeAt()
			if extra != nil {
				extra(u)
			}
			n, err := u.Save(ctx)
			return n > 0, err
		},
	})
	m.Configure(automationrun.StatusActive).
		Permit(triggerComplete, automationrun.StatusCompleted).
		Permit(triggerFail, automationrun.StatusFailed).
		Permit(triggerExit, automationrun.StatusExited)
	return m.FireCtx(ctx, trigger)
}

// Complete ends the Enrollment `completed`. lifecycle.ErrInvalidTransition when it is no
// longer active (an exit won), which the step executor treats as a finished step.
func Complete(ctx context.Context, s *ent.Scoped, runID int64) error {
	return move(ctx, s, runID, triggerComplete, nil)
}

// Fail ends the Enrollment `failed`, keeping the reason. lifecycle.ErrInvalidTransition
// when it is no longer active.
func Fail(ctx context.Context, s *ent.Scoped, runID int64, f Failure) error {
	return move(ctx, s, runID, triggerFail, func(u *ent.AutomationRunScopedUpdate) {
		if f.Kind != "" {
			u.SetFailureKind(f.Kind)
		}
		if f.Status != 0 {
			u.SetFailureStatus(f.Status)
		}
		if f.Excerpt != "" {
			u.SetFailureExcerpt(f.Excerpt)
		}
	})
}
