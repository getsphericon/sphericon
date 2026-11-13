// Package workspacedeletion is the use case of the Workspace deletion cycle (ADR 0043, ADR 0102):
// the deleted Workspaces an Owner may restore, and erasing one once its window has passed.
// Deleting one checks only its typed name, the Workspace's own rule, so it is the module's
// (workspaces.Module.Delete, ADR 0110); the use case answers it, as it lists the deleted ones,
// with whether its Owner is its only one, which the Memberships say. Restoring one answers the
// Workspace summary its Owner reads, so it is internal/usecase/workspaceview's. The cycle's
// table, the window with the erase date and the Workspaces past it are the Workspace's own
// (workspaces.DeletionCycle, workspaces.EraseDate, workspaces.Module.DueForErase), and its erase
// move is the module's unchecked operation (workspacesunchecked.TombstoneIn, ADR 0110), which
// keeps the window and finishes a Workspace no longer pending deletion as a no-op: the erase
// removes the Workspace's rows, then, in the last transaction, its jobs in the queue
// (events.WorkspacePurger), the OAuth codes that name it (oauthclientsunchecked.DeleteCodesIn)
// and writes the tombstone.
//
// The use case takes the raw client: it lists Workspaces that have no scope yet (the Owners'
// deleted Workspaces) and opens a transaction per erase step.
// Each erase step lists a batch of the Workspace's rows of one entity and deletes them itself
// through *ent.Scoped, whichever module owns the table, the Enterprise Edition's included
// (ADR 0110): no rule of an owner governs the rows of a Workspace past its window.
package workspacedeletion

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase is the Workspace deletion cycle over the raw client and the event bus.
type UseCase struct {
	client     *ent.Client
	bus        *events.Bus
	workspaces *workspaces.Module
	now        func() time.Time
	batch      int
	steps      []eraseStep
}

// New returns the use case over the Workspaces, which delete one; now is the Instance clock the
// window is measured on.
func New(client *ent.Client, bus *events.Bus, ws *workspaces.Module, now func() time.Time) *UseCase {
	return &UseCase{client: client, bus: bus, workspaces: ws, now: now, batch: eraseBatch, steps: eraseSteps()}
}

// DeletedWorkspace is a Workspace in its deletion window, and whether it has one Owner alone: no
// other Owner could restore it.
type DeletedWorkspace struct {
	Workspace *ent.Workspace
	OnlyOwner bool
}

// Deleted lists the Workspaces in their deletion window whose User's Role holds workspace:delete
// (their Owners, ADR 0061), most recently deleted first, each with whether the User is its only
// Owner.
func (m *UseCase) Deleted(ctx context.Context, userID int64) ([]DeletedWorkspace, error) {
	held, err := m.client.Membership.Query().
		Where(membership.UserID(userID), memberships.HoldersOf(memberships.PermWorkspaceDelete),
			membership.HasWorkspaceWith(workspaces.InsideDeletionWindow(m.now()), workspace.DiscardedAtNotNil())).
		WithWorkspace().
		Order(membership.ByWorkspaceField(workspace.FieldDiscardedAt, sql.OrderDesc()), ent.Desc(membership.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return m.withSoleOwnership(ctx, lo.Map(held, func(ms *ent.Membership, _ int) *ent.Workspace { return ms.Edges.Workspace }))
}

// Delete deletes the Workspace of s once typedName matches its name (workspaces.Module.Delete) and
// answers it with whether its Owner is its only one.
func (m *UseCase) Delete(ctx context.Context, s *ent.Scoped, actor events.Actor, typedName string) (DeletedWorkspace, error) {
	w, err := m.workspaces.Delete(ctx, s, actor, typedName)
	if err != nil {
		return DeletedWorkspace{}, err
	}
	deleted, err := m.withSoleOwnership(ctx, []*ent.Workspace{w})
	if err != nil {
		return DeletedWorkspace{}, err
	}
	return deleted[0], nil
}

// withSoleOwnership pairs each Workspace with whether it has exactly one Owner. Asked of the
// Workspaces a User owns, it names the ones that have no other Owner to restore them.
func (m *UseCase) withSoleOwnership(ctx context.Context, ws []*ent.Workspace) ([]DeletedWorkspace, error) {
	owners, err := m.client.Membership.Query().
		Where(membership.WorkspaceIDIn(lo.Map(ws, func(w *ent.Workspace, _ int) int64 { return w.ID })...), memberships.OwnerRole()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	perWorkspace := lo.CountValues(lo.Map(owners, func(o *ent.Membership, _ int) int64 { return o.WorkspaceID }))
	return lo.Map(ws, func(w *ent.Workspace, _ int) DeletedWorkspace {
		return DeletedWorkspace{Workspace: w, OnlyOwner: perWorkspace[w.ID] == 1}
	}), nil
}
