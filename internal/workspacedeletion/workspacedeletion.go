// Package workspacedeletion owns the Workspace deletion cycle (ADR 0043): deleting a
// Workspace, restoring it inside its window, the window and erase date, and the move to
// erased. The cycle is a lifecycle table (ADR 0046): active, pending_deletion, erased.
//
// The state is derived from two columns, so no column is added: active is a Workspace with no
// discarded_at, pending_deletion has discarded_at and no erased_at, erased has erased_at. The
// window is not a state: restoring is legal only while discarded_at is inside it, and that is
// part of the update's condition, so the database decides a race with the erase job.
//
// The package takes the raw client: it lists Workspaces that have no scope yet (the Owners'
// deleted Workspaces, the ones whose window has passed), opens a transaction per erase step
// and writes the row the cycle moves.
// It imports neither the holds package nor the outbound package (ent's outboundmessage entity
// is only a row the erase removes); holds and the site read the window from here.
package workspacedeletion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/qmuntal/stateless"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/accounts/roles"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/validation"
)

// DeletionWindow is how long a deleted Workspace stays restorable before its data is
// erased (ADR 0043). A constant, not configuration.
const DeletionWindow = 30 * 24 * time.Hour

// EraseDate is when the data of a Workspace deleted at discardedAt is erased.
func EraseDate(discardedAt time.Time) time.Time { return discardedAt.Add(DeletionWindow) }

// ErrNameMismatch refuses deleting a Workspace whose typed name differs from the stored one.
var ErrNameMismatch = validation.RefuseField("name", "workspacedeletion: the typed name does not match the workspace name", "errors.workspace_name_mismatch")

// ErrNotRestorable refuses restoring a Workspace past its deletion window or already erased. It
// wraps the lifecycle.ErrInvalidTransition the cycle refused the move with.
var ErrNotRestorable = validation.Conflict("workspacedeletion: the workspace can no longer be restored", "errors.workspace_not_restorable")

// State is where a Workspace is in the deletion cycle.
type State string

const (
	StateActive          State = "active"
	StatePendingDeletion State = "pending_deletion"
	StateErased          State = "erased"
)

// Trigger is a move of the cycle.
type Trigger string

const (
	TriggerDelete  Trigger = "delete"
	TriggerRestore Trigger = "restore"
	TriggerErase   Trigger = "erase"
)

// Module is the Workspace deletion cycle over the raw client and the event bus.
type Module struct {
	client *ent.Client
	bus    *events.Bus
	batch  int
}

// New returns the module.
func New(client *ent.Client, bus *events.Bus) *Module {
	return &Module{client: client, bus: bus, batch: eraseBatch}
}

// pendingDeletion is a Workspace deleted and not yet erased, whatever the window says.
func pendingDeletion() predicate.Workspace {
	return workspace.And(workspace.DiscardedAtNotNil(), workspace.ErasedAtIsNil())
}

// insideWindow is a Workspace in its deletion window at now: pending and not past it.
func insideWindow(now time.Time) predicate.Workspace {
	return workspace.And(workspace.DiscardedAtGT(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil())
}

// Due lists the Workspaces whose window has passed at now and that are not erased yet.
func (m *Module) Due(ctx context.Context, now time.Time) ([]int64, error) {
	return m.client.Workspace.Query().
		Where(workspace.DiscardedAtLTE(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil()).
		Order(workspace.ByID()).
		IDs(ctx)
}

// stateOf is the cycle state a Workspace row is in.
func stateOf(ws *ent.Workspace) State {
	switch {
	case ws.ErasedAt != nil:
		return StateErased
	case ws.DiscardedAt != nil:
		return StatePendingDeletion
	default:
		return StateActive
	}
}

// inState is the condition a row is in the state.
func inState(state State) predicate.Workspace {
	switch state {
	case StateActive:
		return accounts.Live()
	case StatePendingDeletion:
		return pendingDeletion()
	default:
		return workspace.ErasedAtNotNil()
	}
}

// cycleStorage is the cycle's storage over c (the raw client or a transaction's) for Workspace id.
// Each write is the compare-and-swap: one conditional UPDATE whose WHERE is the state read,
// plus, for a restore, the window.
func cycleStorage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[State] {
	return lifecycle.Storage[State]{
		Read: func(ctx context.Context) (State, error) {
			ws, err := c.Workspace.Get(ctx, id)
			if err != nil {
				return "", err
			}
			return stateOf(ws), nil
		},
		Write: func(ctx context.Context, was, to State) (bool, error) {
			u := c.Workspace.Update().Where(workspace.ID(id), inState(was))
			switch {
			case was == StateActive && to == StatePendingDeletion:
				u.SetDiscardedAt(now)
			case was == StatePendingDeletion && to == StateActive:
				u.Where(insideWindow(now)).ClearDiscardedAt()
			case was == StatePendingDeletion && to == StateErased:
				// The tombstone: the row stays, with nothing that identifies the Workspace
				// and a fresh collect key.
				key, err := credentials.GenerateCollectKey()
				if err != nil {
					return false, err
				}
				u.SetName("").SetPostalAddress("").SetCollectKey(key).ClearRetentionDays().SetErasedAt(now)
			default:
				return false, fmt.Errorf("workspacedeletion: no move from %s to %s", was, to)
			}
			n, err := u.Save(ctx)
			return n == 1, err
		},
	}
}

// cycleTable is the table of legal moves over a storage.
func cycleTable(s lifecycle.Storage[State]) *stateless.StateMachine {
	sm := lifecycle.New(s)
	sm.Configure(StateActive).Permit(TriggerDelete, StatePendingDeletion)
	sm.Configure(StatePendingDeletion).
		Permit(TriggerRestore, StateActive).
		Permit(TriggerErase, StateErased)
	sm.Configure(StateErased)
	return sm
}

// deletedDiff is the audit diff of the Workspace's deleted state changing.
func deletedDiff(wasDeleted, isDeleted bool) map[string]any {
	return map[string]any{"deleted": events.Change(wasDeleted, isDeleted)}
}

// Delete deletes the scoped Workspace (ADR 0043): it sets discarded_at, which the send path
// reads as the pending-deletion Hold and the lookups read as "not found". The typed name
// must equal the stored name (ErrNameMismatch otherwise). Deleting an already deleted
// Workspace keeps its first discarded_at. The caller has checked the Owner role. Recorded as
// `workspace.delete` in the same transaction.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, actor events.Actor, typedName string, now time.Time) (*ent.Workspace, error) {
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
		if err := cycleTable(cycleStorage(tx, ws.ID, now)).FireCtx(ctx, TriggerDelete); err != nil {
			return err
		}
		if deleted, err = tx.Workspace.Get(ctx, ws.ID); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceDelete, deletedDiff(false, true)))
	})
	return deleted, err
}

// Deleted lists the Workspaces in their deletion window whose User's Role holds workspace:delete
// (their Owners, ADR 0061), most recently deleted first, each with that Membership and its
// Workspace loaded.
func (m *Module) Deleted(ctx context.Context, userID int64, now time.Time) ([]*ent.Membership, error) {
	return m.client.Membership.Query().
		Where(membership.UserID(userID), accounts.HoldersOf(accounts.PermWorkspaceDelete),
			membership.HasWorkspaceWith(insideWindow(now), workspace.DiscardedAtNotNil())).
		WithWorkspace().
		Order(membership.ByWorkspaceField(workspace.FieldDiscardedAt, sql.OrderDesc()), ent.Desc(membership.FieldID)).
		All(ctx)
}

// SoleOwned says which of the Workspaces workspaceIDs have exactly one Owner. Asked of the
// Workspaces a User owns, it names the ones that have no other Owner to restore them.
func (m *Module) SoleOwned(ctx context.Context, workspaceIDs []int64) (map[int64]bool, error) {
	owners, err := m.client.Membership.Query().
		Where(membership.WorkspaceIDIn(workspaceIDs...), roles.Owners()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	perWorkspace := lo.CountValues(lo.Map(owners, func(o *ent.Membership, _ int) int64 { return o.WorkspaceID }))
	return lo.MapValues(perWorkspace, func(n int, _ int64) bool { return n == 1 }), nil
}

// Restore ends the deletion of the Workspace publicID for a User: it clears discarded_at, which
// lifts only the pending-deletion Hold (a suspension or Staff hold stays). A Workspace whose
// User's Role does not hold workspace:delete (ADR 0061), or one that is not deleted, is
// accounts.ErrWorkspaceNotFound: a deleted Workspace is not exposed to anyone else. A Workspace past its window or
// already erased is ErrNotRestorable (a lifecycle.ErrInvalidTransition). Recorded as `workspace.restore` in the
// same transaction. It returns the restored Workspace.
func (m *Module) Restore(ctx context.Context, userID int64, publicID string, actor events.Actor, now time.Time) (*ent.Workspace, error) {
	// An erased Workspace has no Memberships left (the erase removes them), so the lookup
	// below finds nothing and the API answers 404: no one can be authorized to restore it.
	// lifecycle.ErrInvalidTransition from an erased row is only reachable through the table.
	workspaceID, err := m.client.Workspace.Query().
		Where(workspace.PublicID(publicID), workspace.DiscardedAtNotNil(),
			workspace.HasMembershipsWith(membership.UserID(userID), accounts.HoldersOf(accounts.PermWorkspaceDelete))).
		OnlyID(ctx)
	if ent.IsNotFound(err) {
		return nil, accounts.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, err
	}
	var restored *ent.Workspace
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		err := cycleTable(cycleStorage(tx, workspaceID, now)).FireCtx(ctx, TriggerRestore)
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			return fmt.Errorf("%w: %w", ErrNotRestorable, err)
		}
		if err != nil {
			return err
		}
		ws, err := tx.Workspace.Get(ctx, workspaceID)
		if err != nil {
			return err
		}
		restored = ws
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceRestore, deletedDiff(true, false)))
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}

// tombstone moves the Workspace id from pending deletion to erased inside tx: it writes the
// tombstone and erased_at in one conditional update. A Workspace that is not deleted, or is
// already erased, is lifecycle.ErrInvalidTransition and nothing changes. The caller has
// removed what the Workspace owned.
func tombstone(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return cycleTable(cycleStorage(tx, id, now)).FireCtx(ctx, TriggerErase)
}
