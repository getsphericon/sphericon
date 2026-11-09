// Package admission admits a request made on a User's own credential to the operation it
// addresses (ADR 0061), in one call a surface makes before the operation runs. For an operation of
// a Workspace it resolves the User's Membership of the Workspace the request names
// (memberships.Module.Scope), then admits the User: a required acceptance pending
// (users.Module.CheckAcceptance), then, in a Workspace, its Two-factor requirement
// (secondfactors.Module.CheckRequirement); for an operation of no Workspace it admits the User
// alone. So the refusals come in one order on every surface: a Workspace that does not exist or
// of which the User is no member, then a required acceptance pending, then the Second factor
// required. A token of the User's own (a Connection, a Personal token) is admitted the same way
// to the Membership it names (AdmitMembership). What the User's Role allows is the Permissions' to
// say (memberships.Require), not the admission's.
//
// It holds no client: it composes the identity modules that own the rules.
package admission

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// Module admits the requests made on a User's own credential.
type Module struct {
	memberships *memberships.Module
	users       *users.Module
	factors     *secondfactors.Module
}

// New builds the module: memberships resolves the Workspace a request names, u says whether the
// User has a required acceptance pending and factors whether the Workspace's Two-factor
// requirement withholds them.
func New(ms *memberships.Module, u *users.Module, factors *secondfactors.Module) *Module {
	return &Module{memberships: ms, users: u, factors: factors}
}

// Request is a request made on a User's session, as Admit decides it.
type Request struct {
	// UserID is the User the session is of.
	UserID int64
	// Workspace is the public id of the Workspace the operation addresses; empty for an operation
	// of no Workspace.
	Workspace string
	// StrongSession is a session that passed a Second factor step or began with a Passkey (ADR
	// 0052): it meets the Two-factor requirement by itself.
	StrongSession bool
	// WhileAcceptancePending and WhileWithheld are what the operation's contract lets through: a
	// required acceptance pending, and the Two-factor requirement withholding the User.
	WhileAcceptancePending bool
	WhileWithheld          bool
	// At is when the request is made, the time the Two-factor grace is checked against.
	At time.Time
}

// Admitted is what an admitted request of a Workspace acts through; zero for an operation of no
// Workspace.
type Admitted struct {
	// Scoped is the Workspace's scoped client, acting as the User (memberships.Module.ScopeOf).
	Scoped *ent.Scoped
	// Membership is the User's Membership of the Workspace, its User and Workspace loaded.
	Membership *ent.Membership
}

// Admit admits the request, or refuses it: workspaces.ErrNotFound for a Workspace that does not
// exist or of which the User is no member, then users.ErrAcceptancePending and
// secondfactors.ErrSecondFactorRequired, each unless the request lets it through.
func (m *Module) Admit(ctx context.Context, r Request) (Admitted, error) {
	var admitted Admitted
	if r.Workspace != "" {
		s, ms, err := m.memberships.Scope(ctx, r.UserID, r.Workspace)
		if err != nil {
			return Admitted{}, err
		}
		admitted = Admitted{Scoped: s, Membership: ms}
	}
	if err := m.admit(ctx, r, admitted.Membership); err != nil {
		return Admitted{}, err
	}
	return admitted, nil
}

// AdmitMembership admits a request made at the time at on a token of a User's own (a Connection,
// or a Personal token) to the Workspace of ms, the Membership the token names, its Workspace
// loaded: refused with users.ErrAcceptancePending, then secondfactors.ErrSecondFactorRequired, as
// the User's own session would be. A token is never a strong session, and no operation lets it through.
func (m *Module) AdmitMembership(ctx context.Context, ms *ent.Membership, at time.Time) error {
	return m.admit(ctx, Request{UserID: ms.UserID, At: at}, ms)
}

// admit admits the User of r: their required acceptance pending, then, in the Workspace of ms (nil
// for an operation of no Workspace), its Two-factor requirement, each unless r lets it through.
func (m *Module) admit(ctx context.Context, r Request, ms *ent.Membership) error {
	if !r.WhileAcceptancePending {
		if err := m.users.CheckAcceptance(ctx, r.UserID); err != nil {
			return err
		}
	}
	if ms == nil || r.WhileWithheld || r.StrongSession {
		return nil
	}
	return m.factors.CheckRequirement(ctx, ms.UserID, ms.Edges.Workspace.SecondFactorRequiredAt, ms.CreatedAt, r.At)
}
