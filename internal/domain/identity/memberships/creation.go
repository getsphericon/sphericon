package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
)

// The unique constraints of the tables this package writes (ADR 0106).
var _ = db.Declare(
	db.Fault("membership_user_id_workspace_id", "AddInvitedIn writes with ON CONFLICT DO NOTHING, CreateWorkspaceIn's Workspace is new, and AddByDomainIn runs only for a User the Join decision found without a Membership"),
)

// StartingSendCap answers the Send cap a Workspace created now starts with, read on tx, the
// transaction that creates it (suspension.DefaultSendCap.StartingIn, ADR 0067, ADR 0146): nil for
// none. suspension sits above this module in identity, so the caller hands it in.
type StartingSendCap interface {
	StartingIn(ctx context.Context, tx *ent.Client) (*int, error)
}

// ErrCreatorUnverified refuses a Workspace whose creator's email is not proven yet (ADR 0030), so
// sign-up verification is not bypassed by creating Workspaces first.
var ErrCreatorUnverified = validation.Unverified("memberships: the creator's email is not verified", i18n.ErrorsEmailVerificationRequired)

// CreateWorkspaceIn creates a Workspace named name in tx, the transaction of the bus it is bound to,
// with a fresh Workspace id and the Send cap the module's StartingSendCap says it starts with, its
// creator as its Owner (source creator, ADR 0043), and records `workspace.create` in its Audit log,
// the creator its actor: a Workspace is reached through its Memberships, so it is never created
// without its Owner. The name is held to the name rule (workspaces.CreateIn). An Unverified creator
// is ErrCreatorUnverified (ADR 0030) and nothing is written.
func (ms *Module) CreateWorkspaceIn(ctx context.Context, tx *ent.Client, creator *ent.User, name string) (*ent.Workspace, error) {
	if creator.EmailVerifiedAt == nil {
		return nil, ErrCreatorUnverified
	}
	sendCap, err := ms.startingCap.StartingIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	ws, err := workspaces.CreateIn(ctx, tx, name, workspaceid.New, sendCap)
	if err != nil {
		return nil, err
	}
	err = tx.Membership.Create().
		SetUserID(creator.ID).
		SetWorkspaceID(ws.ID).
		SetRole(membership.RoleOwner).
		SetSource(membership.SourceCreator).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	actor := events.Actor{Kind: events.ActorUser, ID: publicid.Encode(creator.ID), Name: creator.Name}
	if err := events.RecordAudit(ctx, ms.pub(tx), events.WorkspaceAudit(ws, actor, events.ActionWorkspaceCreate, nil)); err != nil {
		return nil, err
	}
	return ws, nil
}

// AddInvitedIn gives the User userID a Membership with role in the scoped Workspace, the source an
// accepted Invitation, inside the transaction ts is bound to. A Membership the User already holds
// is left as it is: nothing is written and the error is sql.ErrNoRows (DO NOTHING rather than a
// unique violation, which would abort the transaction).
func (ms *Module) AddInvitedIn(ctx context.Context, ts *ent.Scoped, userID int64, role membership.Role) error {
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
func (ms *Module) AddByDomainIn(ctx context.Context, ts *ent.Scoped, userID int64) (*ent.Membership, error) {
	return ts.Membership().Create().
		SetUserID(userID).
		SetRole(membership.RoleMember).
		SetSource(membership.SourceDomain).
		Save(ctx)
}

// SetRoleIn sets the role of the Membership id in the scoped Workspace, inside the transaction ts
// is bound to; the caller records the change.
func (ms *Module) SetRoleIn(ctx context.Context, ts *ent.Scoped, id int64, role membership.Role) error {
	return ts.Membership().UpdateOneID(id).SetRole(role).Exec(ctx)
}
