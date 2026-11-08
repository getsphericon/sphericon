// Package workspacedeletion is the use case of the Workspace deletion cycle (ADR 0043, ADR 0102):
// restoring a deleted Workspace inside its window, and erasing it once the window has passed.
// Deleting one checks only its typed name, the Workspace's own rule, so it is the module's
// (workspaces.Module.Delete, ADR 0110). The cycle's table and the window with the erase date are
// the Workspace's own (workspaces.DeletionCycle, workspaces.DeletionWindow), and its restore and
// erase moves are the module's unchecked operations (workspacesunchecked.RestoreIn and
// TombstoneIn, ADR 0110): a restore needs a Membership that holds the delete Permission, and the
// erase removes the Workspace's rows, then, in the last transaction, its jobs in the queue
// (events.WorkspacePurger), the OAuth codes that name it (oauthclientsunchecked.DeleteCodesIn) and
// writes the tombstone.
//
// The use case takes the raw client: it lists Workspaces that have no scope yet (the Owners'
// deleted Workspaces, the ones whose window has passed) and opens a transaction per erase step.
// Each erase step lists a batch of the Workspace's rows of one entity and deletes them itself
// through *ent.Scoped, whichever module owns the table, the Enterprise Edition's included
// (ADR 0110): no rule of an owner governs the rows of a Workspace past its window.
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
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces/workspacesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrNotRestorable refuses restoring a Workspace past its deletion window or already erased. It
// wraps the lifecycle.ErrInvalidTransition the cycle refused the move with.
var ErrNotRestorable = validation.Conflict("workspacedeletion: the workspace can no longer be restored", i18n.ErrorsWorkspaceNotRestorable)

// UseCase is the Workspace deletion cycle over the raw client and the event bus.
type UseCase struct {
	client *ent.Client
	bus    *events.Bus
	now    func() time.Time
	batch  int
	steps  []eraseStep
}

// New returns the use case; now is the Instance clock the window is measured on.
func New(client *ent.Client, bus *events.Bus, now func() time.Time) *UseCase {
	return &UseCase{client: client, bus: bus, now: now, batch: eraseBatch, steps: eraseSteps()}
}

// RestoreInput is a restore of a deleted Workspace: the Workspace by its public id, the User who
// restores it and the actor the Audit log records.
type RestoreInput struct {
	UserID   int64
	PublicID string
	Actor    events.Actor
}

// Due lists the Workspaces whose window has passed and that are not erased yet.
func (m *UseCase) Due(ctx context.Context) ([]int64, error) {
	return m.due(ctx, m.now())
}

func (m *UseCase) due(ctx context.Context, now time.Time) ([]int64, error) {
	return m.client.Workspace.Query().
		Where(workspace.DiscardedAtLTE(now.Add(-workspaces.DeletionWindow)), workspace.ErasedAtIsNil()).
		Order(workspace.ByID()).
		IDs(ctx)
}

// Deleted lists the Workspaces in their deletion window whose User's Role holds workspace:delete
// (their Owners, ADR 0061), most recently deleted first, each with that Membership and its
// Workspace loaded.
func (m *UseCase) Deleted(ctx context.Context, userID int64) ([]*ent.Membership, error) {
	return m.client.Membership.Query().
		Where(membership.UserID(userID), memberships.HoldersOf(memberships.PermWorkspaceDelete),
			membership.HasWorkspaceWith(workspaces.InsideDeletionWindow(m.now()), workspace.DiscardedAtNotNil())).
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

// Restore ends the deletion of the Workspace in.PublicID for a User: it clears discarded_at, which
// lifts only the pending-deletion Hold (a suspension or Staff hold stays). A Workspace whose
// User's Role does not hold workspace:delete (ADR 0061), or one that is not deleted, is
// workspaces.ErrNotFound: a deleted Workspace is not exposed to anyone else. A Workspace past its window or
// already erased is ErrNotRestorable (a lifecycle.ErrInvalidTransition). Recorded as `workspace.restore` in the
// same transaction. It returns the restored Workspace.
func (m *UseCase) Restore(ctx context.Context, in RestoreInput) (*ent.Workspace, error) {
	userID, publicID, actor, now := in.UserID, in.PublicID, in.Actor, m.now()
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
		err := workspacesunchecked.RestoreIn(ctx, tx, workspaceID, now)
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
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceRestore, map[string]any{"deleted": events.Change(true, false)}))
	})
	if err != nil {
		return nil, err
	}
	return restored, nil
}
