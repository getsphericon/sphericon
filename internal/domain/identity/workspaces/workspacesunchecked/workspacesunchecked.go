// Package workspacesunchecked holds the workspaces module's unchecked operations (ADR 0110): the
// creation of a Workspace, which refuses an Unverified User and writes the creator's Owner
// Membership on the same transaction, and the restore and the erase of the deletion cycle, which
// need a Membership that holds the delete Permission and every owner's rows removed. Their checks
// are the use cases' (internal/usecase/workspaceview, which creates and restores one,
// internal/usecase/signup and internal/usecase/workspacedeletion); only a use case may import this package. The workspaces
// module does not import it; it declares the cycle's table (workspaces.DeletionCycle), whose
// restore and erase moves are written here.
package workspacesunchecked

import (
	"context"
	"errors"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// errPublicIDsTaken is a generator of Workspace ids that drew only taken ones, publicIDAttempts in
// a row: one taken id is rare already (workspaceid.New), so the generator is broken.
var errPublicIDsTaken = errors.New("workspaces: every workspace id drawn is taken")

// publicIDAttempts is how many Workspace ids CreateIn draws before it gives up.
const publicIDAttempts = 3

// CreateIn writes a new Workspace named name in tx, with a Workspace id newPublicID draws (again
// when the one drawn is taken, a fault after publicIDAttempts draws), a fresh collect key and the
// Send cap it starts with (ADR 0067; nil for none). A Workspace is reached through its Memberships,
// so the caller writes its Owner Membership on the same transaction (membershipsunchecked.AddCreatorIn):
// the use cases internal/usecase/workspaceview and internal/usecase/signup, which refuse an
// Unverified User first (ADR 0030, ADR 0043).
func CreateIn(ctx context.Context, tx *ent.Client, name string, newPublicID func() string, sendCap *int) (*ent.Workspace, error) {
	collectKey, err := credentials.GenerateCollectKey()
	if err != nil {
		return nil, err
	}
	for range publicIDAttempts {
		id, err := tx.Workspace.Create().
			SetName(name).
			SetPublicID(newPublicID()).
			SetCollectKey(collectKey).
			SetNillableSendCap(sendCap).
			OnConflict(entsql.ConflictColumns(workspace.FieldPublicID)).
			DoNothing().
			ID(ctx)
		inserted, err := db.Inserted(err)
		switch {
		case err != nil:
			return nil, err
		case inserted:
			return tx.Workspace.Get(ctx, id)
		}
	}
	return nil, errPublicIDsTaken
}

// RestoreIn moves the Workspace id from pending deletion back to active inside tx, only while
// it is inside its deletion window at now. One past it, already erased or not deleted is
// lifecycle.ErrInvalidTransition and nothing changes.
func RestoreIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return workspaces.DeletionCycle(cycleStorage(tx, id, now)).FireCtx(ctx, workspaces.TriggerRestore)
}

// TombstoneIn moves the Workspace id from pending deletion to erased inside tx: it writes the
// tombstone and erased_at in one conditional update. A Workspace that is not deleted, or is
// already erased, is lifecycle.ErrInvalidTransition and nothing changes. The caller has
// removed what the Workspace owned.
func TombstoneIn(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return workspaces.DeletionCycle(cycleStorage(tx, id, now)).FireCtx(ctx, workspaces.TriggerErase)
}

// cycleStorage is the cycle's storage of the restore and the erase over c (the raw client or a
// transaction's) for Workspace id. Each write is the compare-and-swap: one conditional UPDATE whose
// WHERE is the state read, plus, for a restore, the window.
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
