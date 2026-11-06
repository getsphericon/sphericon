package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// AddCreatorIn writes, in tx, the Owner Membership of the User who creates the Workspace
// workspaceID (source creator, ADR 0043), on the transaction that creates the Workspace
// (workspaces.CreateIn): the use case internal/usecase/workspacecreation, which refuses an
// Unverified User first (ADR 0030).
func AddCreatorIn(ctx context.Context, tx *ent.Client, userID, workspaceID int64) error {
	_, err := tx.Membership.Create().
		SetUserID(userID).
		SetWorkspaceID(workspaceID).
		SetRole(membership.RoleOwner).
		SetSource(membership.SourceCreator).
		Save(ctx)
	return err
}
