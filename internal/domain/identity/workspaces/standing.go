package workspaces

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The Workspace's sending standing lives on its row: the suspension, the abuse exemption, the
// Send cap (internal/domain/identity/suspension says what they mean and audits each change) and the markers of the
// abuse detector's warning and of the suspension's notice (ee/domain/insights/abusedetector,
// internal/usecase/noticedelivery). These functions
// are their writes; each runs on the client or transaction it is given.

// Standing is the suspension state of a Workspace (ADR 0026), kept as suspended_at: a machine
// (ADR 0109) whose two moves are Suspend and Unsuspend.
type Standing string

const (
	StandingActive    Standing = "active"
	StandingSuspended Standing = "suspended"
)

type standingTrigger string

const (
	triggerSuspend   standingTrigger = "suspend"
	triggerUnsuspend standingTrigger = "unsuspend"
)

// suspension is what a Suspend writes with the moment: who did it and why.
type suspension struct {
	kind   workspace.SuspendedByKind
	byID   *string
	reason string
}

// moveStanding fires trigger on the standing of the Workspace id inside tx and reports whether it
// moved. A Workspace already in the state the trigger leads to stays as it is, its first actor and
// reason standing, and so does one that a concurrent move changed first or that does not exist.
func moveStanding(ctx context.Context, tx *ent.Client, id int64, trigger standingTrigger, by suspension, at time.Time) (bool, error) {
	moved := false
	machine := lifecycle.New(lifecycle.Storage[Standing]{
		Read: func(ctx context.Context) (Standing, error) {
			row, err := tx.Workspace.Query().Where(workspace.ID(id)).Select(workspace.FieldSuspendedAt).Only(ctx)
			if err != nil {
				return "", err
			}
			if row.SuspendedAt != nil {
				return StandingSuspended, nil
			}
			return StandingActive, nil
		},
		Write: func(ctx context.Context, from, to Standing) (bool, error) {
			upd := tx.Workspace.Update().Where(workspace.ID(id))
			if from == StandingSuspended {
				upd.Where(workspace.SuspendedAtNotNil())
			} else {
				upd.Where(workspace.SuspendedAtIsNil())
			}
			if to == StandingSuspended {
				upd.SetSuspendedAt(at).SetSuspendedByKind(by.kind).SetNillableSuspendedByID(by.byID).
					SetSuspensionReason(by.reason)
			} else {
				upd.ClearSuspendedAt().ClearSuspendedByKind().ClearSuspendedByID().ClearSuspensionReason().
					SetClearedAt(at)
			}
			n, err := upd.Save(ctx)
			moved = n == 1
			return moved, err
		},
	})
	machine.Configure(StandingActive).Permit(triggerSuspend, StandingSuspended).Ignore(triggerUnsuspend)
	machine.Configure(StandingSuspended).Permit(triggerUnsuspend, StandingActive).Ignore(triggerSuspend)
	err := machine.FireCtx(ctx, trigger)
	if ent.IsNotFound(err) || errors.Is(err, lifecycle.ErrInvalidTransition) {
		return false, nil
	}
	return moved, err
}

// SuspendIn suspends the Workspace id inside tx while it is not suspended: it sets suspended_at to
// `at` with who did it (kind, and byID for a Staff member) and why. It reports whether it did:
// false when the Workspace was already suspended, whose first actor and reason stand.
func SuspendIn(ctx context.Context, tx *ent.Client, id int64, kind workspace.SuspendedByKind, byID *string, reason string, at time.Time) (bool, error) {
	return moveStanding(ctx, tx, id, triggerSuspend, suspension{kind: kind, byID: byID, reason: reason}, at)
}

// UnsuspendIn lifts the suspension of the Workspace id inside tx while it is suspended: it clears
// the suspension and its attribution and stamps cleared_at with `at`. It reports whether it did:
// false when the Workspace was not suspended.
func UnsuspendIn(ctx context.Context, tx *ent.Client, id int64, at time.Time) (bool, error) {
	return moveStanding(ctx, tx, id, triggerUnsuspend, suspension{}, at)
}

// SetAbuseExemptIn sets the abuse exemption of the Workspace id inside tx while it differs, and
// returns how many rows it changed.
func SetAbuseExemptIn(ctx context.Context, tx *ent.Client, id int64, exempt bool) (int, error) {
	return tx.Workspace.Update().
		Where(workspace.ID(id), workspace.AbuseExempt(!exempt)).
		SetAbuseExempt(exempt).
		Save(ctx)
}

// SetSendCapIn sets the Send cap of the Workspace id inside tx, or clears it with nil. A Send cap
// outside the contract's bounds is refused (CheckSendCap).
func SetSendCapIn(ctx context.Context, tx *ent.Client, id int64, sendCap *int) error {
	if err := CheckSendCap(sendCap); err != nil {
		return err
	}
	upd := tx.Workspace.UpdateOneID(id)
	if sendCap == nil {
		upd.ClearSendCap()
	} else {
		upd.SetSendCap(*sendCap)
	}
	return upd.Exec(ctx)
}

// CheckSendCap refuses a Send cap outside the contract's bounds (bounds.MaxPerDay) on the field
// `perDay`; nil, no Send cap, passes. Every write of a Workspace's Send cap checks it.
func CheckSendCap(sendCap *int) error {
	if sendCap == nil {
		return nil
	}
	return validation.Range("perDay", *sendCap, bounds.MaxPerDayMin, bounds.MaxPerDayMax)
}

// MarkAbuseFreezeNotified stamps, at `at`, that the Owners of the Workspace id were told of its
// suspension.
func MarkAbuseFreezeNotified(ctx context.Context, client *ent.Client, id int64, at time.Time) error {
	return client.Workspace.UpdateOneID(id).SetAbuseFreezeNotifiedAt(at).Exec(ctx)
}

// MarkAbuseWarned records, at, that the Owners of the Workspace id were warned of a breach.
func MarkAbuseWarned(ctx context.Context, client *ent.Client, id int64, at time.Time) error {
	return client.Workspace.UpdateOneID(id).SetAbuseWarningNotifiedAt(at).Exec(ctx)
}

// ClearAbuseWarning ends the warning episode of the Workspace id.
func ClearAbuseWarning(ctx context.Context, client *ent.Client, id int64) error {
	return client.Workspace.UpdateOneID(id).ClearAbuseWarningNotifiedAt().Exec(ctx)
}

// ClearAbuseWarnings ends the warning episode of every Workspace that is warned and matches which.
func ClearAbuseWarnings(ctx context.Context, client *ent.Client, which ...predicate.Workspace) error {
	return client.Workspace.Update().
		Where(append([]predicate.Workspace{workspace.AbuseWarningNotifiedAtNotNil()}, which...)...).
		ClearAbuseWarningNotifiedAt().
		Exec(ctx)
}

// SetFeedPosition gives the Workspaces ids the service feed position (ADR 0062): the write of
// internal/domain/identity/workspacefeed's hook, which takes the position under its lock.
func SetFeedPosition(ctx context.Context, client *ent.Client, position int64, ids ...int64) error {
	return client.Workspace.Update().Where(workspace.IDIn(ids...)).SetFeedPosition(position).Exec(ctx)
}
