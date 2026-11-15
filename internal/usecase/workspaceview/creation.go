package workspaceview

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// CreateInput is a Workspace a Verified User creates: its name, and whether the User's session
// meets the Two-factor requirement by itself (Caller.StrongSession).
type CreateInput struct {
	UserID        int64
	Name          string
	StrongSession bool
}

// Create creates a Workspace named in.Name for a Verified User, who becomes its Owner (ADR 0043),
// and answers its View for them. Names are free text and need not be unique. The Workspace, its
// Owner Membership, the Send cap it starts with and its `workspace.create` Audit entry are
// memberships' (memberships.CreateWorkspaceIn); the use case refuses an Unverified User first
// (users.ErrEmailNotVerified) and answers the View. A User's first Workspace, created by the step
// that makes it Verified, is the Sign-up's (internal/usecase/signup).
func (u *UseCase) Create(ctx context.Context, in CreateInput) (View, error) {
	var ws *ent.Workspace
	err := u.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		usr, err := users.VerifiedIn(ctx, tx, in.UserID)
		if err != nil {
			return err
		}
		ws, err = memberships.CreateWorkspaceIn(ctx, tx, pub, u.defaultSendCap, usr, in.Name)
		return err
	})
	if err != nil {
		return View{}, err
	}
	return u.memberView(ctx, in.UserID, ws.PublicID, events.Actor{}, in.StrongSession)
}

// memberView is the View of the live Workspace publicID for its member userID, whose Membership
// the use case resolves as the scope middleware would (memberships.Module.Scope).
func (u *UseCase) memberView(ctx context.Context, userID int64, publicID string, actor events.Actor, strongSession bool) (View, error) {
	s, m, err := u.memberships.Scope(ctx, userID, publicID)
	if err != nil {
		return View{}, err
	}
	return u.view(ctx, s, Caller{Membership: m, Actor: actor, StrongSession: strongSession}, m.Edges.Workspace)
}
