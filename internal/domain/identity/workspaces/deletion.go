package workspaces

import (
	"context"
	"fmt"
	"time"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// The deletion cycle of a Workspace (ADR 0043) is a lifecycle table (ADR 0046): active,
// pending_deletion, erased. Deleting is this module's operation, checked by the typed name (Delete);
// restoring needs a Membership that holds the delete Permission and erasing every owner's rows, so
// those two moves are unchecked operations of internal/domain/identity/workspaces/workspacesunchecked
// (RestoreIn and TombstoneIn, ADR 0110): the Workspace view (internal/usecase/workspaceview)
// restores, the deletion cycle (internal/usecase/workspacedeletion) erases.
// The table of legal moves is declared here once (DeletionCycle), and each package writes only its
// own moves: a storage's Write is the compare-and-swap of its moves, and the Workspace table is
// written only in this module and its subpackage.
//
// The state is derived from two columns, so no column is added: active is a Workspace with no
// discarded_at, pending_deletion has discarded_at and no erased_at, erased has erased_at. The
// window is not a state: restoring is legal only while discarded_at is inside it, and that is
// part of the update's condition, so the database decides a race with the erase job.

// DeletionState is where a Workspace is in the deletion cycle.
type DeletionState string

// The states of the deletion cycle.
const (
	StateActive          DeletionState = "active"
	StatePendingDeletion DeletionState = "pending_deletion"
	StateErased          DeletionState = "erased"
)

// DeletionTrigger is a move of the cycle.
type DeletionTrigger string

// The moves of the deletion cycle.
const (
	TriggerDelete  DeletionTrigger = "delete"
	TriggerRestore DeletionTrigger = "restore"
	TriggerErase   DeletionTrigger = "erase"
)

// pendingDeletion is a Workspace deleted and not yet erased, whatever the window says.
func pendingDeletion() predicate.Workspace {
	return workspace.And(workspace.DiscardedAtNotNil(), workspace.ErasedAtIsNil())
}

// InsideDeletionWindow is a Workspace in its deletion window at now: pending and not past it.
func InsideDeletionWindow(now time.Time) predicate.Workspace {
	return workspace.And(workspace.DiscardedAtGT(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil())
}

// DeletionStateOf is the cycle state a Workspace row is in.
func DeletionStateOf(ws *ent.Workspace) DeletionState {
	switch {
	case ws.ErasedAt != nil:
		return StateErased
	case ws.DiscardedAt != nil:
		return StatePendingDeletion
	default:
		return StateActive
	}
}

// InDeletionState is the condition a row is in the state: the WHERE of a move's compare-and-swap.
func InDeletionState(state DeletionState) predicate.Workspace {
	switch state {
	case StateActive:
		return Live()
	case StatePendingDeletion:
		return pendingDeletion()
	default:
		return workspace.ErasedAtNotNil()
	}
}

// ReadDeletionState is the Read of a cycle's storage over c (the raw client or a transaction's)
// for Workspace id.
func ReadDeletionState(c *ent.Client, id int64) func(context.Context) (DeletionState, error) {
	return func(ctx context.Context) (DeletionState, error) {
		ws, err := c.Workspace.Get(ctx, id)
		if err != nil {
			return "", err
		}
		return DeletionStateOf(ws), nil
	}
}

// DeletionCycle is the table of legal moves over a storage.
func DeletionCycle(s lifecycle.Storage[DeletionState]) *stateless.StateMachine {
	sm := lifecycle.New(s)
	sm.Configure(StateActive).Permit(TriggerDelete, StatePendingDeletion)
	sm.Configure(StatePendingDeletion).
		Permit(TriggerRestore, StateActive).
		Permit(TriggerErase, StateErased)
	sm.Configure(StateErased)
	return sm
}

// deleteStorage is the cycle's storage of this module's one move, the delete, over c for Workspace
// id: one conditional UPDATE whose WHERE is the state read.
func deleteStorage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[DeletionState] {
	return lifecycle.Storage[DeletionState]{
		Read: ReadDeletionState(c, id),
		Write: func(ctx context.Context, was, to DeletionState) (bool, error) {
			if was != StateActive || to != StatePendingDeletion {
				return false, fmt.Errorf("workspaces: no move from %s to %s", was, to)
			}
			n, err := c.Workspace.Update().Where(workspace.ID(id), InDeletionState(was)).SetDiscardedAt(now).Save(ctx)
			return n == 1, err
		},
	}
}

// ErrNameMismatch refuses deleting a Workspace whose typed name differs from the stored one.
var ErrNameMismatch = validation.RefuseField("name", "workspaces: the typed name does not match the workspace name", i18n.ErrorsWorkspaceNameMismatch)

// Delete deletes the scoped Workspace (ADR 0043): it moves it from active to pending deletion,
// setting discarded_at, which the send path reads as the pending-deletion Hold and the lookups
// read as "not found". The typed name must equal the stored name (ErrNameMismatch otherwise).
// Deleting an already deleted Workspace keeps its first discarded_at, so its window does not
// restart. The caller has checked the Owner role. Recorded as `workspace.delete` in the same
// transaction. It returns the deleted Workspace.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, actor events.Actor, typedName string) (*ent.Workspace, error) {
	now := m.now()
	var deleted *ent.Workspace
	err := m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		ws, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		if typedName != ws.Name {
			return ErrNameMismatch
		}
		if ws.DiscardedAt != nil {
			deleted = ws
			return nil
		}
		if err := DeletionCycle(deleteStorage(tx, ws.ID, now)).FireCtx(ctx, TriggerDelete); err != nil {
			return err
		}
		if deleted, err = tx.Workspace.Get(ctx, ws.ID); err != nil {
			return err
		}
		diff := map[string]any{"deleted": events.Change(false, true)}
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceDelete, diff))
	})
	return deleted, err
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
