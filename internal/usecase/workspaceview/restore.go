package workspaceview

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces/workspacesunchecked"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// ErrNotRestorable refuses restoring a Workspace past its deletion window or already erased. It
// wraps the lifecycle.ErrInvalidTransition the cycle refused the move with.
var ErrNotRestorable = validation.Conflict("workspaceview: the workspace can no longer be restored", i18n.ErrorsWorkspaceNotRestorable)

// RestoreInput is a restore of a deleted Workspace: the Workspace by its public id, the User who
// restores it, the actor the Audit log records and whether the User's session meets the Two-factor
// requirement by itself (Caller.StrongSession).
type RestoreInput struct {
	UserID        int64
	PublicID      string
	Actor         events.Actor
	StrongSession bool
}

// Restore ends the deletion of the Workspace in.PublicID for a User and answers its View for them:
// it clears discarded_at, which lifts only the pending-deletion Hold (a suspension or Staff hold
// stays). A Workspace whose User's Role does not hold workspace:delete (ADR 0061), or one that is
// not deleted, is workspaces.ErrNotFound (memberships.Module.Restorable): a deleted Workspace is
// not exposed to anyone else. A
// Workspace past its window or already erased is ErrNotRestorable (a
// lifecycle.ErrInvalidTransition). Recorded as `workspace.restore` in the same transaction. The
// restore's move is the module's unchecked operation (workspacesunchecked.RestoreIn, ADR 0110): it
// needs a Membership that holds the delete Permission, which the lookup here checks.
func (u *UseCase) Restore(ctx context.Context, in RestoreInput) (View, error) {
	// An erased Workspace has no Memberships left (the erase removes them), so the lookup
	// finds nothing and the API answers 404: no one can be authorized to restore it.
	// lifecycle.ErrInvalidTransition from an erased row is only reachable through the table.
	workspaceID, err := u.memberships.Restorable(ctx, in.UserID, in.PublicID)
	if err != nil {
		return View{}, err
	}
	err = u.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		ws, err := workspacesunchecked.RestoreIn(ctx, tx, workspaceID, u.now())
		if errors.Is(err, lifecycle.ErrInvalidTransition) {
			return fmt.Errorf("%w: %w", ErrNotRestorable, err)
		}
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, in.Actor, events.ActionWorkspaceRestore, map[string]any{"deleted": events.Change(true, false)}))
	})
	if err != nil {
		return View{}, err
	}
	// The Workspace is live again, so the User's Owner Membership resolves like any other.
	return u.memberView(ctx, in.UserID, in.PublicID, in.Actor, in.StrongSession)
}
