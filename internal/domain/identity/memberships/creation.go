package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/infra/db"
)

// The unique constraints of the tables this package writes (ADR 0106).
var _ = db.Declare(
	db.Fault("membership_user_id_workspace_id", "AddInvitedIn writes with ON CONFLICT DO NOTHING, AddCreatorIn's Workspace is new, and AddByDomainIn runs only for a User the Join decision found without a Membership"),
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

// AddInvitedIn gives the User userID a Membership with role in the scoped Workspace, the source an
// accepted Invitation, inside the transaction ts is bound to. A Membership the User already holds
// is left as it is: nothing is written and the error is sql.ErrNoRows (DO NOTHING rather than a
// unique violation, which would abort the transaction).
func AddInvitedIn(ctx context.Context, ts *ent.Scoped, userID int64, role membership.Role) error {
	return ts.Membership().Create().
		SetUserID(userID).
		SetRole(role).
		SetSource(membership.SourceInvitation).
		OnConflictColumns(membership.FieldUserID, membership.FieldWorkspaceID).
		DoNothing().
		Exec(ctx)
}

// AddByDomainIn makes the User userID a member of the scoped Workspace, the source its Join
// domain, inside the transaction ts is bound to.
func AddByDomainIn(ctx context.Context, ts *ent.Scoped, userID int64) (*ent.Membership, error) {
	return ts.Membership().Create().
		SetUserID(userID).
		SetRole(membership.RoleMember).
		SetSource(membership.SourceDomain).
		Save(ctx)
}

// SetRoleIn sets the role of the Membership id in the scoped Workspace, inside the transaction ts
// is bound to; the caller records the change.
func SetRoleIn(ctx context.Context, ts *ent.Scoped, id int64, role membership.Role) error {
	return ts.Membership().UpdateOneID(id).SetRole(role).Exec(ctx)
}
