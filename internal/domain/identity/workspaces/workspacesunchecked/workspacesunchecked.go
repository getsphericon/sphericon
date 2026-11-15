// Package workspacesunchecked holds the workspaces module's unchecked operations (ADR 0110): the
// restore and the erase of the deletion cycle, which need a Membership that holds the delete
// Permission and every owner's rows removed. Their checks are the use cases'
// (internal/usecase/workspaceview, which restores one, and internal/usecase/workspacedeletion); only
// a use case may import this package. The workspaces module does not import it; it declares the
// cycle's table (workspaces.DeletionCycle), whose restore and erase moves are written here.
package workspacesunchecked

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// RestoreIn moves the Workspace id from pending deletion back to active inside tx, only while
// it is inside its deletion window at now, and returns it as it now reads. One past it, already
// erased or not deleted is lifecycle.ErrInvalidTransition and nothing changes.
func RestoreIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) (*ent.Workspace, error) {
	if err := workspaces.DeletionCycle(cycleStorage(tx, id, now)).FireCtx(ctx, workspaces.TriggerRestore); err != nil {
		return nil, err
	}
	return tx.Workspace.Get(ctx, id)
}

// TombstoneIn moves the Workspace id from pending deletion to erased inside tx, only once it is
// past its deletion window at now: it writes the tombstone and erased_at in one conditional
// update, and reports whether it did. One still inside its window is refused
// (lifecycle.ErrInvalidTransition) and nothing changes. One not pending deletion (never deleted,
// restored, or erased already) is its no-op: it reports false with no error (ADR 0145). The
// caller has removed what the Workspace owned.
func TombstoneIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) (erased bool, err error) {
	err = workspaces.DeletionCycle(cycleStorage(tx, id, now)).FireCtx(ctx, workspaces.TriggerErase)
	if !errors.Is(err, lifecycle.ErrInvalidTransition) {
		return err == nil, err
	}
	// The move was not permitted or lost its compare-and-swap, so the state is read again: a
	// Workspace another writer moved meanwhile, as a restore that wrote first, is no longer
	// pending, and one still pending was kept by the window.
	state, err := workspaces.ReadDeletionState(tx, id)(ctx)
	switch {
	case err != nil:
		return false, err
	case state == workspaces.StatePendingDeletion:
		return false, fmt.Errorf("%w: workspace %d is inside its deletion window", lifecycle.ErrInvalidTransition, id)
	default:
		return false, nil
	}
}

// cycleStorage is the cycle's storage of the restore and the erase over c (the raw client or a
// transaction's) for Workspace id. Each write is the compare-and-swap: one conditional UPDATE whose
// WHERE is the state read, plus the window: a restore inside it, a tombstone past it.
func cycleStorage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[workspaces.DeletionState] {
	return lifecycle.Storage[workspaces.DeletionState]{
		Read: workspaces.ReadDeletionState(c, id),
		Write: func(ctx context.Context, was, to workspaces.DeletionState) (bool, error) {
			u := c.Workspace.Update().Where(workspace.ID(id), workspaces.InDeletionState(was))
			switch {
			case was == workspaces.StatePendingDeletion && to == workspaces.StateActive:
				u.Where(workspaces.InsideDeletionWindow(now)).ClearDiscardedAt()
			case was == workspaces.StatePendingDeletion && to == workspaces.StateErased:
				// The tombstone: the row stays, with nothing that identifies the Workspace
				// and a fresh collect key.
				key := credentials.GenerateCollectKey()
				u.Where(workspaces.PastDeletionWindow(now)).SetName("").SetPostalAddress("").SetCollectKey(key).ClearRetentionDays().SetErasedAt(now)
			default:
				return false, fmt.Errorf("workspaces: no move from %s to %s", was, to)
			}
			n, err := u.Save(ctx)
			return n == 1, err
		},
	}
}
