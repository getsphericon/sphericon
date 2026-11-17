// Package memberships owns the Membership of a User in a Workspace (ADR 0061, ADR 0102): the
// lookup that turns a /w/{id} into the Workspace's scope, the Roles and the Permissions they
// hold, the Owners of a Workspace with the one notice to them, the creation of a Workspace with
// its creator as Owner (CreateWorkspaceIn), and the changes of a Role, removal and Leaving under
// the Owner invariant. It sits low in identity, above workspaces, so
// every module can ask a Permission or tell the Owners something.
//
// The module takes the raw client: a User's Memberships span Workspaces (the lookup before any
// scope exists, the list of a User's Workspaces, the Owners of many Workspaces), and the
// notice to the Owners scopes to the Workspace it is given.
package memberships

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

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
	client      *ent.Client
	bus         *events.Bus
	creation    *workspaces.Creation
	startingCap StartingSendCap
	now         func() time.Time
}

// New builds the module over the ent client; the bus opens the transactions of a Role change,
// a removal and Leaving, names the actor of a scope and publishes in the transaction of a client an
// operation is given (events.Bus.PublishIn). creation writes the Workspace CreateWorkspaceIn
// creates, with the Send cap startingCap says it starts with, and now is the Instance clock a
// Second factor reminder is stamped by.
func New(client *ent.Client, bus *events.Bus, creation *workspaces.Creation, startingCap StartingSendCap, now func() time.Time) *Module {
	return &Module{client: client, bus: bus, creation: creation, startingCap: startingCap, now: now}
}

// MemberScope is a User's Membership of a Workspace and the scoped client of that Workspace,
// acting as the User.
type MemberScope struct {
	Scope      *ent.Scoped
	Membership *ent.Membership
}

// Scope resolves the Workspace addressed by a /w/{id} path segment for a User and
// returns its scoped client with the User's Membership (its User and Workspace
// loaded, for the Role and the Two-factor requirement). It is the site's construction
// point of the scoped client: access is "does this User have a Membership on this
// Workspace?". workspaces.ErrNotFound means the Workspace id does not exist or the User is
// not a member.
func (ms *Module) Scope(ctx context.Context, userID int64, publicID string) (MemberScope, error) {
	m, err := ms.client.Membership.Query().
		Where(membership.UserID(userID), membership.HasWorkspaceWith(workspace.PublicID(publicID), workspaces.Live())).
		WithUser().
		WithWorkspace().
		Only(ctx)
	if ent.IsNotFound(err) {
		return MemberScope{}, workspaces.ErrNotFound
	}
	if err != nil {
		return MemberScope{}, err
	}
	return MemberScope{Scope: ms.ScopeOf(m), Membership: m}, nil
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
	actor := events.Actor{Kind: events.ActorUser, ID: publicid.Encode(m.UserID)}
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

// OfUserIn lists every Membership of the User on the transaction tx, a Workspace in its deletion
// window included, oldest Workspace first: the Workspaces a step about the User scopes to in turn.
func (ms *Module) OfUserIn(ctx context.Context, tx *ent.Client, userID int64) ([]*ent.Membership, error) {
	return tx.Membership.Query().
		Where(membership.UserID(userID)).
		Order(ent.Asc(membership.FieldWorkspaceID)).
		All(ctx)
}

// Members lists the Memberships of the Workspace s with their Users, oldest first.
func (ms *Module) Members(ctx context.Context, s *ent.Scoped) ([]*ent.Membership, error) {
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

// ErrSecondFactorResetElsewhere refuses a Second factor reset of a member who also belongs to
// another Workspace where the caller may not reset it (ADR 0177): the factor is the User's, so the
// right to clear it is needed wherever it guards them. It names no Workspace; the caller learns only
// that the member belongs to others, whose Owners or a Staff member reset it.
var ErrSecondFactorResetElsewhere = validation.RefuseField(FieldMember, "memberships: the member belongs to other workspaces where the caller may not reset their second factor", i18n.ErrorsSecondFactorResetElsewhere)

// SecondFactorResetTargetInput is a Second factor reset of a member: the User who resets it, their
// Role, and the Membership whose Second factor is reset.
type SecondFactorResetTargetInput struct {
	CallerID   int64
	CallerRole membership.Role
	TargetID   int64
}

// SecondFactorResetTarget loads the Membership in.TargetID of the Workspace s as the target of a
// Second factor reset by the User in.CallerID holding in.CallerRole there, by the rule of one
// Workspace (resetRight): second_factor:manage, and owners:manage for an Owner's, refused as
// Require refuses them; and nobody resets their own (ErrResetOwnSecondFactor). A target outside the
// Workspace is ErrMemberNotFound. The member's other Workspaces are SecondFactorResetElsewhereIn's.
func (ms *Module) SecondFactorResetTarget(ctx context.Context, s *ent.Scoped, in SecondFactorResetTargetInput) (*ent.Membership, error) {
	target, err := s.Membership().Get(ctx, in.TargetID)
	if err != nil {
		return nil, memberTarget(err)
	}
	if err := resetRight(in.CallerRole, target); err != nil {
		return nil, err
	}
	if target.UserID == in.CallerID {
		return nil, ErrResetOwnSecondFactor
	}
	return target, nil
}

// SecondFactorResetElsewhereInput is a Second factor reset SecondFactorResetTarget allowed: the User
// who resets it and the target Membership it loaded in the Workspace the reset is made in.
type SecondFactorResetElsewhereInput struct {
	CallerID int64
	Target   *ent.Membership
}

// SecondFactorResetElsewhereIn is nil when the User in.CallerID may reset the Second factor of the
// User of in.Target in every other live Workspace that User holds a Membership in, read on the
// transaction tx of the reset (ADR 0177): there the caller holds a Membership whose Role passes the
// rule of one Workspace (resetRight). Otherwise ErrSecondFactorResetElsewhere, whichever Workspace
// refused.
func (ms *Module) SecondFactorResetElsewhereIn(ctx context.Context, tx *ent.Client, in SecondFactorResetElsewhereInput) error {
	held, err := tx.Membership.Query().
		Where(membership.UserID(in.Target.UserID), membership.HasWorkspaceWith(workspaces.Live())).
		All(ctx)
	if err != nil {
		return err
	}
	callers, err := ms.OfUserIn(ctx, tx, in.CallerID)
	if err != nil {
		return err
	}
	callerRoles := make(map[int64]membership.Role, len(callers))
	for _, m := range callers {
		callerRoles[m.WorkspaceID] = m.Role
	}
	for _, m := range held {
		if m.WorkspaceID == in.Target.WorkspaceID {
			continue
		}
		role, member := callerRoles[m.WorkspaceID]
		if !member || resetRight(role, m) != nil {
			return ErrSecondFactorResetElsewhere
		}
	}
	return nil
}

// resetRight is nil when a caller holding role in a Workspace may reset the Second factor of its
// member target there: second_factor:manage, and owners:manage for an Owner's. Otherwise the
// refusal of the Permission the role lacks.
func resetRight(role membership.Role, target *ent.Membership) error {
	if err := Require(role, PermSecondFactorManage); err != nil {
		return err
	}
	if IsOwner(target.Role) {
		return Require(role, PermOwnersManage)
	}
	return nil
}
