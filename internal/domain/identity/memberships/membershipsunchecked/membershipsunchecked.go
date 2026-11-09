// Package membershipsunchecked holds the memberships module's unchecked operations (ADR 0110): the
// creator's Owner Membership, written only with its new Workspace once the creator is known to be
// Verified, and the leave of the only Owner of a Workspace in its deletion window, which only User
// deletion makes after it has refused a User who solely owns a live Workspace (ADR 0050). Their
// checks are the use cases' (internal/usecase/workspacesummary, internal/usecase/signup and
// internal/usecase/userdeletion); only a use case may import this package. The memberships module
// does not import it.
package membershipsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// AddCreatorIn writes, in tx, the Owner Membership of the User who creates the Workspace
// workspaceID (source creator, ADR 0043), on the transaction that creates the Workspace
// (workspacesunchecked.CreateIn): the use cases internal/usecase/workspacesummary and
// internal/usecase/signup, which refuse an Unverified User first (ADR 0030).
func AddCreatorIn(ctx context.Context, tx *ent.Client, userID, workspaceID int64) error {
	_, err := tx.Membership.Create().
		SetUserID(userID).
		SetWorkspaceID(workspaceID).
		SetRole(membership.RoleOwner).
		SetSource(membership.SourceCreator).
		Save(ctx)
	return err
}

// LeaveDoomedWorkspaceIn removes User userID's own Membership like memberships.LeaveIn but lets the
// only Owner go too, for User deletion of a Workspace that is in its deletion window (ADR 0050):
// nobody is left to hand it to, and the Workspace is erased on schedule. The User is the actor of
// the `membership.delete` Audit entry.
func LeaveDoomedWorkspaceIn(ctx context.Context, ts *ent.Scoped, pub events.Publisher, userID int64) error {
	target, _, actor, err := memberships.LockSelfIn(ctx, ts, userID)
	if err != nil {
		return err
	}
	if err := ts.Membership().DeleteOneID(target.ID).Exec(ctx); err != nil {
		return err
	}
	return events.RecordAudit(ctx, pub, memberships.RemovalAudit(ts.WorkspaceID(), actor, target))
}
