package staffmembers

import (
	"context"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	entstaffmember "github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// enrolment is a Staff member's TOTP enrolment, kept as totp_confirmed_at: a machine (ADR 0109).
// The first accepted code confirms it; a reset (`sphericon staff reset-totp`) returns it to pending
// from either state, and the Staff member enrols again at the next login.
type enrolment string

const (
	enrolmentPending   enrolment = "pending"
	enrolmentConfirmed enrolment = "confirmed"
)

type enrolmentTrigger string

const (
	triggerConfirm enrolmentTrigger = "confirm"
	triggerReset   enrolmentTrigger = "reset"
)

func enrolmentOf(op *ent.StaffMember) enrolment {
	if op.TotpConfirmedAt != nil {
		return enrolmentConfirmed
	}
	return enrolmentPending
}

// enrolledAs is the condition that the Staff member's enrolment is still e: the compare-and-swap of
// a move from e.
func enrolledAs(e enrolment) predicate.StaffMember {
	if e == enrolmentConfirmed {
		return entstaffmember.TotpConfirmedAtNotNil()
	}
	return entstaffmember.TotpConfirmedAtIsNil()
}

// enrolmentMachine is the enrolment's table over storage. A reset of a pending enrolment re-enters
// it, so its Write still runs: it drops the pending key and ends the sessions.
func enrolmentMachine(storage lifecycle.Storage[enrolment]) *stateless.StateMachine {
	m := lifecycle.New(storage)
	m.Configure(enrolmentPending).
		Permit(triggerConfirm, enrolmentConfirmed).
		PermitReentry(triggerReset)
	m.Configure(enrolmentConfirmed).
		Permit(triggerReset, enrolmentPending)
	return m
}

// confirmEnrolment confirms the pending enrolment of op on tx with the first accepted step: it
// stamps totp_confirmed_at and bumps the session epoch.
func (m *Module) confirmEnrolment(ctx context.Context, tx *ent.Tx, op *ent.StaffMember, step int64) error {
	machine := enrolmentMachine(lifecycle.Storage[enrolment]{
		Read: func(context.Context) (enrolment, error) { return enrolmentOf(op), nil },
		Write: func(ctx context.Context, from, _ enrolment) (bool, error) {
			n, err := tx.StaffMember.Update().
				Where(entstaffmember.ID(op.ID), enrolledAs(from)).
				SetTotpLastStep(step).
				SetTotpConfirmedAt(m.now()).
				AddSessionEpoch(1).
				Save(ctx)
			return n == 1, err
		},
	})
	return machine.FireCtx(ctx, triggerConfirm)
}
