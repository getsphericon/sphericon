package automations

import (
	"context"
	"errors"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// The Enrollment's lifecycle (ADR 0046): it starts `active` and leaves it exactly once, for
// `completed`, `failed` or `exited`; all three are final. Every move is a compare-and-swap
// from `active`, so whichever writer reaches the row first decides a race, and a late step
// cannot overwrite an exit (the move reports lifecycle.ErrInvalidTransition instead).
type trigger string

const (
	triggerComplete trigger = "complete"
	triggerFail     trigger = "fail"
	triggerExit     trigger = "exit"
)

// Failure is what an Enrollment keeps of the step that failed it. The zero value records
// nothing beyond the `failed` status.
type Failure struct {
	Kind    automationrun.FailureKind
	Status  int
	Excerpt string
}

// FailureOfDelivery is the record of a call to a third party that failed: the kind of the
// delivery error, with the answer's status and excerpt where the receiver gave one. An error that
// is not a delivery error is a network failure.
func FailureOfDelivery(sendErr error) Failure {
	var delivery *webhook.DeliveryError
	if !errors.As(sendErr, &delivery) {
		delivery = &webhook.DeliveryError{Kind: webhook.KindNetwork}
	}
	f := Failure{Kind: automationrun.FailureKind(delivery.Kind), Excerpt: delivery.Excerpt}
	if delivery.Kind == webhook.KindHTTPStatus {
		f.Status = delivery.Status
	}
	return f
}

// machine is the Enrollment's table over its stored status. extra adds fields to the write.
func machine(s *ent.Scoped, runID int64, extra func(*ent.AutomationRunScopedUpdate)) *stateless.StateMachine {
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
	return m
}

// move fires t on one Enrollment, making the move out of `active` a conditional update.
func move(ctx context.Context, s *ent.Scoped, runID int64, t trigger, extra func(*ent.AutomationRunScopedUpdate)) error {
	return machine(s, runID, extra).FireCtx(ctx, t)
}

// IsActive reports, from a fresh read through the lifecycle table, whether the Enrollment
// can still move: a step that finds it exited, completed or failed stops quietly. A run
// that no longer exists (erased with its Contact) is not active either.
func IsActive(ctx context.Context, s *ent.Scoped, runID int64) (bool, error) {
	ok, err := machine(s, runID, nil).CanFireCtx(ctx, triggerExit)
	if ent.IsNotFound(err) {
		return false, nil
	}
	return ok, err
}

// Stale reports that err means the Enrollment is no longer there to move: another writer
// took it out of `active` (an exit won) or it was erased with its Contact. A step that gets
// it finishes quietly; nil and every other error are not stale.
func Stale(err error) bool {
	return errors.Is(err, lifecycle.ErrInvalidTransition) || ent.IsNotFound(err)
}

// Advance moves an active Enrollment standing on step from to step to, parking it until resumeAt
// (nil: no wait). It is a compare-and-swap on `active` and on the current step, so a step cannot
// overwrite an exit and a job delivered twice cannot move the Enrollment twice:
// lifecycle.ErrInvalidTransition when the Enrollment is no longer active or no longer on from.
func Advance(ctx context.Context, s *ent.Scoped, runID int64, from, to int, resumeAt *time.Time) error {
	u := s.AutomationRun().Update().
		Where(automationrun.ID(runID), automationrun.StatusEQ(automationrun.StatusActive), automationrun.CurrentStep(from)).
		SetCurrentStep(to)
	if resumeAt != nil {
		u.SetResumeAt(*resumeAt)
	} else {
		u.ClearResumeAt()
	}
	n, err := u.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return lifecycle.ErrInvalidTransition
	}
	return nil
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
