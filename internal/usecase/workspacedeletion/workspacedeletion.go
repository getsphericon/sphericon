// Package workspacedeletion is the use case of the Workspace deletion cycle (ADR 0043, ADR 0102):
// deleting a Workspace, restoring it inside its window, and erasing it once the window has passed.
// The cycle's table and the window with the erase date are the Workspace's own (workspaces.DiscardIn,
// RestoreIn, TombstoneIn, workspaces.DeletionWindow); the erase removes the Workspace's rows,
// then, in the last transaction, the OAuth codes that name it (oauthclients.DeleteCodesIn) and
// writes the tombstone.
//
// The use case takes the raw client: it lists Workspaces that have no scope yet (the Owners'
// deleted Workspaces, the ones whose window has passed) and opens a transaction per erase step.
// It imports neither the holds package nor the outbound package (ent's outboundmessage entity
// is only a row the erase removes).
package workspacedeletion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/memberships"
	"github.com/getsphericon/sphericon/internal/workspaces"
)

// ErrNameMismatch refuses deleting a Workspace whose typed name differs from the stored one.
var ErrNameMismatch = validation.RefuseField("name", "workspacedeletion: the typed name does not match the workspace name", i18n.ErrorsWorkspaceNameMismatch)

// ErrNotRestorable refuses restoring a Workspace past its deletion window or already erased. It
// wraps the lifecycle.ErrInvalidTransition the cycle refused the move with.
var ErrNotRestorable = validation.Conflict("workspacedeletion: the workspace can no longer be restored", i18n.ErrorsWorkspaceNotRestorable)

// UseCase is the Workspace deletion cycle over the raw client and the event bus.
type UseCase struct {
	client *ent.Client
	bus    *events.Bus
	batch  int
}

// New returns the use case.
func New(client *ent.Client, bus *events.Bus) *UseCase {
	return &UseCase{client: client, bus: bus, batch: eraseBatch}
}

// Due lists the Workspaces whose window has passed at now and that are not erased yet.
func (m *UseCase) Due(ctx context.Context, now time.Time) ([]int64, error) {
	return m.client.Workspace.Query().
		Where(workspace.DiscardedAtLTE(now.Add(-workspaces.DeletionWindow)), workspace.ErasedAtIsNil()).
		Order(workspace.ByID()).
		IDs(ctx)
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
func (m *UseCase) Delete(ctx context.Context, s *ent.Scoped, actor events.Actor, typedName string, now time.Time) (*ent.Workspace, error) {
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
		if err := workspaces.DiscardIn(ctx, tx, ws.ID, now); err != nil {
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
func (m *UseCase) Deleted(ctx context.Context, userID int64, now time.Time) ([]*ent.Membership, error) {
	return m.client.Membership.Query().
		Where(membership.UserID(userID), memberships.HoldersOf(memberships.PermWorkspaceDelete),
			membership.HasWorkspaceWith(workspaces.InsideDeletionWindow(now), workspace.DiscardedAtNotNil())).
		WithWorkspace().
		Order(membership.ByWorkspaceField(workspace.FieldDiscardedAt, sql.OrderDesc()), ent.Desc(membership.FieldID)).
		All(ctx)
}

// SoleOwned says which of the Workspaces workspaceIDs have exactly one Owner. Asked of the
// Workspaces a User owns, it names the ones that have no other Owner to restore them.
func (m *UseCase) SoleOwned(ctx context.Context, workspaceIDs []int64) (map[int64]bool, error) {
	owners, err := m.client.Membership.Query().
		Where(membership.WorkspaceIDIn(workspaceIDs...), memberships.OwnerRole()).
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
// workspaces.ErrNotFound: a deleted Workspace is not exposed to anyone else. A Workspace past its window or
// already erased is ErrNotRestorable (a lifecycle.ErrInvalidTransition). Recorded as `workspace.restore` in the
// same transaction. It returns the restored Workspace.
func (m *UseCase) Restore(ctx context.Context, userID int64, publicID string, actor events.Actor, now time.Time) (*ent.Workspace, error) {
	// An erased Workspace has no Memberships left (the erase removes them), so the lookup
	// below finds nothing and the API answers 404: no one can be authorized to restore it.
	// lifecycle.ErrInvalidTransition from an erased row is only reachable through the table.
	workspaceID, err := m.client.Workspace.Query().
		Where(workspace.PublicID(publicID), workspace.DiscardedAtNotNil(),
			workspace.HasMembershipsWith(membership.UserID(userID), memberships.HoldersOf(memberships.PermWorkspaceDelete))).
		OnlyID(ctx)
	if ent.IsNotFound(err) {
		return nil, workspaces.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var restored *ent.Workspace
	err = m.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		err := workspaces.RestoreIn(ctx, tx, workspaceID, now)
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
