package automations

import (
	"context"
	"errors"
	"time"

	"github.com/qmuntal/stateless"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// The Enrollment's lifecycle (ADR 0046): it starts `active` and leaves it exactly once, for
// `completed`, `failed`, `exited` or `goal_reached`; all four are final. Every move is a compare-and-swap
// from `active`, so whichever writer reaches the row first decides a race, and a late step
// cannot overwrite an exit (the move reports lifecycle.ErrInvalidTransition instead).
type trigger string

const (
	triggerComplete trigger = "complete"
	triggerFail     trigger = "fail"
	triggerExit     trigger = "exit"
	// triggerReachGoal ends the Enrollment when its version's goal event is recorded (#751).
	triggerReachGoal trigger = "reach_goal"
)

// failure is what an Enrollment keeps of the step that failed it. The zero value records
// nothing beyond the `failed` status.
type failure struct {
	Kind    automationrun.FailureKind
	Status  int
	Excerpt string
}

// failureOfDelivery is the record of a call to a third party that failed: the kind of the
// delivery error, with the answer's status and excerpt where the receiver gave one. An error that
// is not a delivery error is a network failure.
func failureOfDelivery(sendErr error) failure {
	var delivery *webhook.DeliveryError
	if !errors.As(sendErr, &delivery) {
		delivery = &webhook.DeliveryError{Kind: webhook.KindNetwork}
	}
	f := failure{Kind: automationrun.FailureKind(delivery.Kind), Excerpt: delivery.Excerpt}
	if delivery.Kind == webhook.KindHTTPStatus {
		f.Status = delivery.Status
	}
	return f
}

// machine is the Enrollment's table over its stored status. A move out of `active` ends it at
// endedAt; extra adds fields to the write.
func machine(s *ent.Scoped, runID int64, endedAt time.Time, extra func(*ent.AutomationRunScopedUpdate)) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[automationrun.Status]{
		Read: func(ctx context.Context) (automationrun.Status, error) {
			run, err := s.AutomationRun().Get(ctx, runID)
			if err != nil {
				return "", err
			}
			return run.Status, nil
		},
		Write: func(ctx context.Context, from, to automationrun.Status) (bool, error) {
			// Every state but `active` is final, so this write is the Enrollment's one move out of
			// it, and the moment it ended is stamped with it.
			u := s.AutomationRun().Update().
				Where(automationrun.ID(runID), automationrun.StatusEQ(from)).
				SetStatus(to).
				SetEndedAt(endedAt).
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
		Permit(triggerExit, automationrun.StatusExited).
		Permit(triggerReachGoal, automationrun.StatusGoalReached)
	return m
}

// endingEvents is the System event each end of an Enrollment records (#897). A `failed` Enrollment
// records none: it is an operational failure, not something the Contact did.
var endingEvents = map[trigger]string{
	triggerComplete:  systemevent.AutomationCompleted,
	triggerExit:      systemevent.AutomationExited,
	triggerReachGoal: systemevent.AutomationGoalReached,
}

// move fires t on one Enrollment, making the move out of `active` a conditional update that
// ends it at endedAt: the time of the Event that decided the end when an Event did (an exit or
// goal Event), else the caller's clock. The end is recorded as its System event through pub, in
// the transaction of the move, so it is published iff this writer made the move.
func move(ctx context.Context, s *ent.Scoped, pub events.Publisher, runID int64, t trigger, endedAt time.Time, extra func(*ent.AutomationRunScopedUpdate)) error {
	if err := machine(s, runID, endedAt, extra).FireCtx(ctx, t); err != nil {
		return err
	}
	action, recorded := endingEvents[t]
	if !recorded {
		return nil
	}
	run, err := s.AutomationRun().Query().Where(automationrun.ID(runID)).WithContact().Only(ctx)
	if err != nil {
		return err
	}
	return publishEnrollment(ctx, s, pub, action, run)
}

// publishEnrollment records the System event action of the Enrollment run, whose Contact edge is
// loaded.
func publishEnrollment(ctx context.Context, s *ent.Scoped, pub events.Publisher, action string, run *ent.AutomationRun) error {
	var email string
	if c := run.Edges.Contact; c != nil {
		email = lo.FromPtr(c.Email)
	}
	return pub.Publish(ctx, &events.EnrollmentChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: run.ContactID, Email: email, AutomationID: run.AutomationID,
	})
}

// isActive reports, from a fresh read through the lifecycle table, whether the Enrollment
// can still move: a step that finds it exited, completed or failed stops quietly. A run
// that no longer exists (erased with its Contact) is not active either.
func isActive(ctx context.Context, s *ent.Scoped, runID int64) (bool, error) {
	ok, err := machine(s, runID, time.Time{}, nil).CanFireCtx(ctx, triggerExit)
	if ent.IsNotFound(err) {
		return false, nil
	}
	return ok, err
}

// stale reports that err means the Enrollment is no longer there to move: another writer
// took it out of `active` (an exit won) or it was erased with its Contact. A step that gets
// it finishes quietly; nil and every other error are not stale.
func stale(err error) bool {
	return errors.Is(err, lifecycle.ErrInvalidTransition) || ent.IsNotFound(err)
}

// advanceRun moves an active Enrollment standing on step from to step to, parking it until resumeAt
// (nil: no wait). It is a compare-and-swap on `active` and on the current step, so a step cannot
// overwrite an exit and a job delivered twice cannot move the Enrollment twice:
// lifecycle.ErrInvalidTransition when the Enrollment is no longer active or no longer on from.
func advanceRun(ctx context.Context, s *ent.Scoped, runID int64, from, to int, resumeAt *time.Time) error {
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

// completeRun ends the Enrollment `completed` at now, the caller's clock, and records
// automation.completed through pub. lifecycle.ErrInvalidTransition when it is no longer active (an
// exit won), which the step executor treats as a finished step.
func completeRun(ctx context.Context, s *ent.Scoped, pub events.Publisher, runID int64, now time.Time) error {
	return move(ctx, s, pub, runID, triggerComplete, now, nil)
}

// failRun ends the Enrollment `failed` at now, the caller's clock, keeping the reason. It records no
// System event. lifecycle.ErrInvalidTransition when it is no longer active.
func failRun(ctx context.Context, s *ent.Scoped, runID int64, now time.Time, f failure) error {
	return move(ctx, s, nil, runID, triggerFail, now, func(u *ent.AutomationRunScopedUpdate) {
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
