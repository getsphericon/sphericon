package workspaces

import (
	"context"
	"fmt"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The deletion cycle of a Workspace (ADR 0043) is a lifecycle table (ADR 0046): active,
// pending_deletion, erased. The use case internal/usecase/workspacedeletion moves a Workspace
// through it; the row's moves are this module's, as the row is.
//
// The state is derived from two columns, so no column is added: active is a Workspace with no
// discarded_at, pending_deletion has discarded_at and no erased_at, erased has erased_at. The
// window is not a state: restoring is legal only while discarded_at is inside it, and that is
// part of the update's condition, so the database decides a race with the erase job.

// deletionState is where a Workspace is in the deletion cycle.
type deletionState string

const (
	stateActive          deletionState = "active"
	statePendingDeletion deletionState = "pending_deletion"
	stateErased          deletionState = "erased"
)

// deletionTrigger is a move of the cycle.
type deletionTrigger string

const (
	triggerDelete  deletionTrigger = "delete"
	triggerRestore deletionTrigger = "restore"
	triggerErase   deletionTrigger = "erase"
)

// pendingDeletion is a Workspace deleted and not yet erased, whatever the window says.
func pendingDeletion() predicate.Workspace {
	return workspace.And(workspace.DiscardedAtNotNil(), workspace.ErasedAtIsNil())
}

// InsideDeletionWindow is a Workspace in its deletion window at now: pending and not past it.
func InsideDeletionWindow(now time.Time) predicate.Workspace {
	return workspace.And(workspace.DiscardedAtGT(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil())
}

// stateOf is the cycle state a Workspace row is in.
func stateOf(ws *ent.Workspace) deletionState {
	switch {
	case ws.ErasedAt != nil:
		return stateErased
	case ws.DiscardedAt != nil:
		return statePendingDeletion
	default:
		return stateActive
	}
}

// inState is the condition a row is in the state.
func inState(state deletionState) predicate.Workspace {
	switch state {
	case stateActive:
		return Live()
	case statePendingDeletion:
		return pendingDeletion()
	default:
		return workspace.ErasedAtNotNil()
	}
}

// cycleStorage is the cycle's storage over c (the raw client or a transaction's) for Workspace id.
// Each write is the compare-and-swap: one conditional UPDATE whose WHERE is the state read,
// plus, for a restore, the window.
func cycleStorage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[deletionState] {
	return lifecycle.Storage[deletionState]{
		Read: func(ctx context.Context) (deletionState, error) {
			ws, err := c.Workspace.Get(ctx, id)
			if err != nil {
				return "", err
			}
			return stateOf(ws), nil
		},
		Write: func(ctx context.Context, was, to deletionState) (bool, error) {
			u := c.Workspace.Update().Where(workspace.ID(id), inState(was))
			switch {
			case was == stateActive && to == statePendingDeletion:
				u.SetDiscardedAt(now)
			case was == statePendingDeletion && to == stateActive:
				u.Where(InsideDeletionWindow(now)).ClearDiscardedAt()
			case was == statePendingDeletion && to == stateErased:
				// The tombstone: the row stays, with nothing that identifies the Workspace
				// and a fresh collect key.
				key, err := credentials.GenerateCollectKey()
				if err != nil {
					return false, err
				}
				u.SetName("").SetPostalAddress("").SetCollectKey(key).ClearRetentionDays().SetErasedAt(now)
			default:
				return false, fmt.Errorf("workspaces: no move from %s to %s", was, to)
			}
			n, err := u.Save(ctx)
			return n == 1, err
		},
	}
}

// cycleTable is the table of legal moves over a storage.
func cycleTable(s lifecycle.Storage[deletionState]) *stateless.StateMachine {
	sm := lifecycle.New(s)
	sm.Configure(stateActive).Permit(triggerDelete, statePendingDeletion)
	sm.Configure(statePendingDeletion).
		Permit(triggerRestore, stateActive).
		Permit(triggerErase, stateErased)
	sm.Configure(stateErased)
	return sm
}

// DiscardIn moves the Workspace id from active to pending deletion inside tx: it sets
// discarded_at to now. A Workspace that is not active is lifecycle.ErrInvalidTransition.
func DiscardIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return cycleTable(cycleStorage(tx, id, now)).FireCtx(ctx, triggerDelete)
}

// RestoreIn moves the Workspace id from pending deletion back to active inside tx, only while
// it is inside its deletion window at now. One past it, already erased or not deleted is
// lifecycle.ErrInvalidTransition and nothing changes.
func RestoreIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return cycleTable(cycleStorage(tx, id, now)).FireCtx(ctx, triggerRestore)
}

// TombstoneIn moves the Workspace id from pending deletion to erased inside tx: it writes the
// tombstone and erased_at in one conditional update. A Workspace that is not deleted, or is
// already erased, is lifecycle.ErrInvalidTransition and nothing changes. The caller has
// removed what the Workspace owned.
func TombstoneIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return cycleTable(cycleStorage(tx, id, now)).FireCtx(ctx, triggerErase)
}

// Erased reports whether the Workspace id is erased: its row is a tombstone, or there is none. A
// job of such a Workspace has nothing left to work on and must write nothing into it.
func Erased(ctx context.Context, client *ent.Client, id int64) (bool, error) {
	live, err := client.Workspace.Query().Where(workspace.ID(id), workspace.ErasedAtIsNil()).Exist(ctx)
	if err != nil {
		return false, err
	}
	return !live, nil
}
