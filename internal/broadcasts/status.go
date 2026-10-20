package broadcasts

import (
	"context"
	"errors"
	"fmt"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// trigger is a move of the Broadcast lifecycle (ADR 0046).
type trigger string

const (
	triggerSchedule   trigger = "schedule"   // draft or scheduled -> scheduled, at a time
	triggerUnschedule trigger = "unschedule" // scheduled -> draft
	triggerSend       trigger = "send"       // draft or scheduled -> sending (the claim)
	triggerPlan       trigger = "plan"       // draft or scheduled -> sending; a re-plan of sending converges
	triggerResume     trigger = "resume"     // scheduled -> sending; a re-run of sending converges (a queued job)
	triggerRevert     trigger = "revert"     // scheduled or sending -> draft (the enqueue failed)
	triggerFinalize   trigger = "finalize"   // sending -> sent
	triggerFail       trigger = "fail"       // sending -> failed
)

// update is the extra columns a move writes together with the status.
type update func(*ent.BroadcastScopedUpdate)

// machine is the Broadcast transition table: every status write of a Broadcast goes through
// it and is a compare-and-swap on the status it read. Plan is legal only before the send
// (draft, scheduled) and, as a convergent re-plan, while sending; from sent or failed it is
// an invalid transition, so a late or retried job cannot regress a finished Broadcast. Resume
// is the plan of a queued job, enqueued after the claim: it is not legal from draft, so an
// unschedule that lands after the job's Due check is not undone by it.
func machine(s *ent.Scoped, id int64, set update) *stateless.StateMachine {
	m := lifecycle.New(lifecycle.Storage[broadcast.Status]{
		Read: func(ctx context.Context) (broadcast.Status, error) {
			b, err := s.Broadcast().Query().Where(broadcast.ID(id)).Only(ctx)
			if ent.IsNotFound(err) {
				return "", ErrNotFound
			}
			if err != nil {
				return "", err
			}
			return b.Status, nil
		},
		Write: func(ctx context.Context, from, to broadcast.Status) (bool, error) {
			u := s.Broadcast().Update().
				Where(broadcast.ID(id), broadcast.StatusEQ(from)).
				SetStatus(to)
			if set != nil {
				set(u)
			}
			n, err := u.Save(ctx)
			return n > 0, err
		},
	})
	m.Configure(broadcast.StatusDraft).
		Permit(triggerSchedule, broadcast.StatusScheduled).
		Permit(triggerSend, broadcast.StatusSending).
		Permit(triggerPlan, broadcast.StatusSending)
	m.Configure(broadcast.StatusScheduled).
		PermitReentry(triggerSchedule).
		Permit(triggerUnschedule, broadcast.StatusDraft).
		Permit(triggerSend, broadcast.StatusSending).
		Permit(triggerPlan, broadcast.StatusSending).
		Permit(triggerResume, broadcast.StatusSending).
		Permit(triggerRevert, broadcast.StatusDraft)
	m.Configure(broadcast.StatusSending).
		PermitReentry(triggerPlan).
		PermitReentry(triggerResume).
		Permit(triggerRevert, broadcast.StatusDraft).
		Permit(triggerFinalize, broadcast.StatusSent).
		Permit(triggerFail, broadcast.StatusFailed)
	m.Configure(broadcast.StatusSent)
	m.Configure(broadcast.StatusFailed)
	return m
}

// fire makes one move of the Broadcast, writing set with the status. An unpermitted move, or
// one that lost the race, is lifecycle.ErrInvalidTransition; an unknown Broadcast is ErrNotFound.
func fire(ctx context.Context, s *ent.Scoped, id int64, t trigger, set update) error {
	return machine(s, id, set).FireCtx(ctx, t)
}

// begin moves a Broadcast to sending for its plan, by trigger t (triggerPlan for a direct
// caller, triggerResume for a queued job). A claimable Broadcast is claimed; one already
// sending stays (a re-plan converges on the same recipient rows). A sent or failed one, and a
// draft for triggerResume, is lifecycle.ErrInvalidTransition and keeps its status.
func begin(ctx context.Context, s *ent.Scoped, id int64, t trigger) error {
	return fire(ctx, s, id, t, nil)
}

// canBegin reports begin's verdict without writing: nil, ErrNotFound, or
// lifecycle.ErrInvalidTransition.
func canBegin(ctx context.Context, s *ent.Scoped, id int64, t trigger) error {
	m := machine(s, id, nil)
	ok, err := m.CanFireCtx(ctx, t)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s is not permitted from the Broadcast's status", lifecycle.ErrInvalidTransition, t)
	}
	return nil
}

// fail moves a sending Broadcast to failed.
func fail(ctx context.Context, s *ent.Scoped, id int64) error {
	return fire(ctx, s, id, triggerFail, nil)
}

// notSendable maps a refused claim to the API's 409 sentinel, keeping the lifecycle error in the chain.
func notSendable(err error) error {
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return fmt.Errorf("%w: %w", ErrNotSendable, err)
	}
	return err
}
