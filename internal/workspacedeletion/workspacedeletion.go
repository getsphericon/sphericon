// Package workspacedeletion owns the Workspace deletion cycle (ADR 0043): deleting a
// Workspace, restoring it inside its window, the window and erase date, and the move to
// erased. The cycle is a lifecycle table (ADR 0046): active, pending_deletion, erased.
//
// The state is derived from two columns, so no column is added: active is a Workspace with no
// deleted_at, pending_deletion has deleted_at and no erased_at, erased has erased_at. The
// window is not a state: restoring is legal only while deleted_at is inside it, and that is
// part of the update's condition, so the database decides a race with the erase job.
//
// The package takes the raw client: it lists Workspaces that have no scope yet (the Owners'
// deleted Workspaces, the ones whose window has passed) and writes the row the cycle moves.
// It imports neither holds nor Outbound; holds and the site read the window from here.
package workspacedeletion

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/lifecycle"
)

// DeletionWindow is how long a deleted Workspace stays restorable before its data is
// erased (ADR 0043). A constant, not configuration.
const DeletionWindow = 30 * 24 * time.Hour

// EraseDate is when the data of a Workspace deleted at deletedAt is erased.
func EraseDate(deletedAt time.Time) time.Time { return deletedAt.Add(DeletionWindow) }

// ErrNameMismatch refuses deleting a Workspace whose typed name differs from the stored one.
var ErrNameMismatch = errors.New("workspacedeletion: the typed name does not match the workspace name")

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
}

// New returns the module.
func New(client *ent.Client, bus *events.Bus) *Module {
	return &Module{client: client, bus: bus}
}

// pending is a Workspace deleted and not yet erased, whatever the window says.
func pending() predicate.Workspace {
	return workspace.And(workspace.DeletedAtNotNil(), workspace.ErasedAtIsNil())
}

// insideWindow is a Workspace in its deletion window at now: pending and not past it.
func insideWindow(now time.Time) predicate.Workspace {
	return workspace.And(workspace.DeletedAtGT(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil())
}

// Due lists the Workspaces whose window has passed at now and that are not erased yet.
func (m *Module) Due(ctx context.Context, now time.Time) ([]int64, error) {
	return m.client.Workspace.Query().
		Where(workspace.DeletedAtLTE(now.Add(-DeletionWindow)), workspace.ErasedAtIsNil()).
		Order(workspace.ByID()).
		IDs(ctx)
}

// stateOf is the cycle state a Workspace row is in.
func stateOf(ws *ent.Workspace) State {
	switch {
	case ws.ErasedAt != nil:
		return StateErased
	case ws.DeletedAt != nil:
		return StatePendingDeletion
	default:
		return StateActive
	}
}

// from is the condition a row is in the state.
func from(state State) predicate.Workspace {
	switch state {
	case StateActive:
		return workspace.DeletedAtIsNil()
	case StatePendingDeletion:
		return pending()
	default:
		return workspace.ErasedAtNotNil()
	}
}

// storage is the cycle's storage over c (the raw client or a transaction's) for Workspace id.
// Each write is the compare-and-swap: one conditional UPDATE whose WHERE is the state read,
// plus, for a restore, the window.
func storage(c *ent.Client, id int64, now time.Time) lifecycle.Storage[State] {
	return lifecycle.Storage[State]{
		Read: func(ctx context.Context) (State, error) {
			ws, err := c.Workspace.Get(ctx, id)
			if err != nil {
				return "", err
			}
			return stateOf(ws), nil
		},
		Write: func(ctx context.Context, was, to State) (bool, error) {
			u := c.Workspace.Update().Where(workspace.ID(id), from(was))
			switch {
			case was == StateActive && to == StatePendingDeletion:
				u.SetDeletedAt(now)
			case was == StatePendingDeletion && to == StateActive:
				u.Where(insideWindow(now)).ClearDeletedAt()
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

// machine is the transition table over a storage.
func machine(s lifecycle.Storage[State]) *stateless.StateMachine {
	sm := lifecycle.New(s)
	sm.Configure(StateActive).Permit(TriggerDelete, StatePendingDeletion)
	sm.Configure(StatePendingDeletion).
		Permit(TriggerRestore, StateActive).
		Permit(TriggerErase, StateErased)
	sm.Configure(StateErased)
	return sm
}

// workspaceAudit is the Audit entry of a Workspace deletion action: the Workspace is its
// own target.
func workspaceAudit(ws *ent.Workspace, actor events.Actor, action string, diff map[string]any) *events.AuditEntry {
	return &events.AuditEntry{
		WorkspaceID: ws.ID,
		Actor:       actor,
		Action:      action,
		TargetType:  workspace.Label,
		TargetID:    strconv.FormatInt(ws.ID, 10),
		TargetName:  ws.Name,
		Diff:        diff,
	}
}

// deletedDiff is the audit diff of the Workspace's deleted state changing.
func deletedDiff(from, to bool) map[string]any {
	return map[string]any{"deleted": map[string]any{"from": from, "to": to}}
}

// Delete deletes the scoped Workspace (ADR 0043): it sets deleted_at, which the send path
// reads as the pending-deletion Hold and the lookups read as "not found". The typed name
// must equal the stored name (ErrNameMismatch otherwise). Deleting an already deleted
// Workspace keeps its first deleted_at. The caller has checked the Owner role. Recorded as
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
		if ws.DeletedAt != nil {
			deleted = ws
			return nil
		}
		if err := machine(storage(tx, ws.ID, now)).FireCtx(ctx, TriggerDelete); err != nil {
			return err
		}
		if deleted, err = tx.Workspace.Get(ctx, ws.ID); err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, workspaceAudit(ws, actor, events.ActionWorkspaceDelete, deletedDiff(false, true)))
	})
	return deleted, err
}

// Deleted lists the Workspaces the User owns that are in their deletion window, most
// recently deleted first, each with its Owner Membership and Workspace loaded.
func (m *Module) Deleted(ctx context.Context, userID int64, now time.Time) ([]*ent.Membership, error) {
	return m.client.Membership.Query().
		Where(membership.UserID(userID), membership.RoleEQ(membership.RoleOwner),
			membership.HasWorkspaceWith(insideWindow(now), workspace.DeletedAtNotNil())).
		WithWorkspace().
		Order(membership.ByWorkspaceField(workspace.FieldDeletedAt, sql.OrderDesc()), ent.Desc(membership.FieldID)).
		All(ctx)
}

// Restore ends the deletion of the Workspace publicID for a User: it clears deleted_at, which
// lifts only the pending-deletion Hold (a suspension or Staff hold stays). A Workspace the
// User has no Owner Membership on, or one that is not deleted, is an ent NotFound error: a
// deleted Workspace is not exposed to anyone but its Owners. A Workspace past its window or
// already erased is lifecycle.ErrInvalidTransition. Recorded as `workspace.restore` in the
// same transaction. The returned Membership carries its User and the restored Workspace.
func (m *Module) Restore(ctx context.Context, userID int64, publicID string, actor events.Actor, now time.Time) (*ent.Membership, error) {
	mem, err := m.client.Membership.Query().
		Where(membership.UserID(userID), membership.RoleEQ(membership.RoleOwner),
			membership.HasWorkspaceWith(workspace.PublicID(publicID), workspace.DeletedAtNotNil())).
		WithUser().
		Only(ctx)
	if err != nil {
		return nil, err
	}
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		if err := machine(storage(tx, mem.WorkspaceID, now)).FireCtx(ctx, TriggerRestore); err != nil {
			return err
		}
		ws, err := tx.Workspace.Get(ctx, mem.WorkspaceID)
		if err != nil {
			return err
		}
		mem.Edges.Workspace = ws
		return events.RecordAudit(ctx, pub, workspaceAudit(ws, actor, events.ActionWorkspaceRestore, deletedDiff(true, false)))
	})
	if err != nil {
		return nil, err
	}
	return mem, nil
}

// Erase moves the Workspace id from pending deletion to erased inside tx: it writes the
// tombstone and erased_at in one conditional update. A Workspace that is not deleted, or is
// already erased, is lifecycle.ErrInvalidTransition and nothing changes. The caller has
// removed what the Workspace owned.
func (m *Module) Erase(ctx context.Context, tx *ent.Client, id int64, now time.Time) error {
	return machine(storage(tx, id, now)).FireCtx(ctx, TriggerErase)
}
