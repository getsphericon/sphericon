// Package memberships owns the Membership of a User in a Workspace (ADR 0061, ADR 0102): the
// lookup that turns a /w/{id} into the Workspace's scope, the Roles and the Permissions they
// hold, the Owners of a Workspace with the one notice to them, and the changes of a Role,
// removal and Leaving under the Owner invariant. It sits low in identity, above workspaces, so
// every module can ask a Permission or tell the Owners something.
//
// The module takes the raw client: a User's Memberships span Workspaces (the lookup before any
// scope exists, the list of a User's Workspaces, the Owners of many Workspaces), and the
// notice to the Owners scopes to the Workspace it is given.
package memberships

import (
	"context"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Module reads and writes Memberships over the raw client.
type Module struct {
	client *ent.Client
	bus    *events.Bus
}

// New builds the module over the ent client; the bus opens the transactions of a Role change,
// a removal and Leaving, and names the actor of a scope.
func New(client *ent.Client, bus *events.Bus) *Module {
	return &Module{client: client, bus: bus}
}

// Scope resolves the Workspace addressed by a /w/{id} path segment for a User and
// returns its scoped client with the User's Membership (its User and Workspace
// loaded, for the Role and the Two-factor requirement). It is the site's construction
// point of the scoped client: access is "does this User have a Membership on this
// Workspace?". workspaces.ErrNotFound means the Workspace id does not exist or the User is
// not a member.
func (ms *Module) Scope(ctx context.Context, userID int64, publicID string) (*ent.Scoped, *ent.Membership, error) {
	m, err := ms.client.Membership.Query().
		Where(membership.UserID(userID), membership.HasWorkspaceWith(workspace.PublicID(publicID), workspaces.Live())).
		WithUser().
		WithWorkspace().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil, workspaces.ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return ms.ScopeOf(m), m, nil
}

// Restorable is the id of the deleted Workspace publicID whose User's Role holds workspace:delete
// (its Owners, ADR 0061): the Membership a restore needs. Any other Workspace, live, of which the
// User is no member, whose User's Role does not hold the Permission, or already erased (the erase
// removes the Memberships), is workspaces.ErrNotFound: a deleted Workspace is not exposed to
// anyone else. The deletion window is not asked: a Workspace past it is found, and its restore
// refused by the cycle.
func (ms *Module) Restorable(ctx context.Context, userID int64, publicID string) (int64, error) {
	m, err := ms.client.Membership.Query().
		Where(membership.UserID(userID), HoldersOf(PermWorkspaceDelete),
			membership.HasWorkspaceWith(workspace.PublicID(publicID), workspaces.Deleted())).
		Only(ctx)
	if ent.IsNotFound(err) {
		return 0, workspaces.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return m.WorkspaceID, nil
}

// ScopeOf is the scoped client of a loaded Membership's Workspace, acting as its User
// (the User edge, when loaded, names the actor). It is Scope without the lookup, for a
// caller that already holds the Membership.
func (ms *Module) ScopeOf(m *ent.Membership) *ent.Scoped {
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(m.UserID, 10)}
	if u := m.Edges.User; u != nil {
		actor.Name = u.Name
	}
	return ms.bus.Act(ms.client.Scoped(m.WorkspaceID), actor)
}

// OfUser lists a User's Memberships with their User and Workspace loaded, oldest
// Workspace first.
func (ms *Module) OfUser(ctx context.Context, userID int64) ([]*ent.Membership, error) {
	return ms.client.Membership.Query().
		Where(membership.UserID(userID), membership.HasWorkspaceWith(workspaces.Live())).
		WithUser().
		WithWorkspace().
		Order(ent.Asc(membership.FieldWorkspaceID)).
		All(ctx)
}

// Members lists the Memberships of the Workspace s with their Users, oldest first.
func Members(ctx context.Context, s *ent.Scoped) ([]*ent.Membership, error) {
	return s.Membership().Query().
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// ErrResetOwnSecondFactor refuses a caller resetting their own Second factor as a member's: they
// disable it in their account, which takes a password and a code.
// FieldMember and FieldWorkspace are what a refusal of the addressed member, or of the Workspace
// itself, is reported under: no request field holds either (the member is the path's id), so the
// app shows such a refusal's detail, not a field's error.
const (
	FieldMember    = "member"
	FieldWorkspace = "workspace"
)

var ErrResetOwnSecondFactor = validation.RefuseField(FieldMember, "memberships: cannot reset your own second factor", i18n.ErrorsSecondFactorResetSelf)

// SecondFactorResetTarget loads the Membership targetID of the Workspace s as the target of a
// Second factor reset by the User callerID holding callerRole: resetting an Owner's needs
// owners:manage, refused as Require refuses it, and nobody resets their own
// (ErrResetOwnSecondFactor). A target outside the Workspace is ErrMemberNotFound.
func SecondFactorResetTarget(ctx context.Context, s *ent.Scoped, callerID int64, callerRole membership.Role, targetID int64) (*ent.Membership, error) {
	target, err := s.Membership().Get(ctx, targetID)
	if err != nil {
		return nil, memberTarget(err)
	}
	if IsOwner(target.Role) {
		if err := Require(callerRole, PermOwnersManage); err != nil {
			return nil, err
		}
	}
	if target.UserID == callerID {
		return nil, ErrResetOwnSecondFactor
	}
	return target, nil
}
