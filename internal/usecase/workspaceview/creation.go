package workspaceview

import (
	"context"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships/membershipsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces/workspacesunchecked"
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
// and answers its View for them. Names are free text and need not be unique. The Workspace, the
// Owner Membership with the Send cap it starts with (ADR 0067) and the `workspace.create` Audit
// entry are written atomically: a Workspace is reached through Memberships, so one without its
// owner row would be inaccessible. An Unverified User gets users.ErrEmailNotVerified. The name is
// held to the name rule by the Workspace's own write (workspacesunchecked.CreateIn). A User's first
// Workspace, created by the step that makes it Verified, is the Sign-up's (internal/usecase/signup),
// which composes the same two modules on that step's transaction.
func (u *UseCase) Create(ctx context.Context, in CreateInput) (View, error) {
	name := in.Name
	var ws *ent.Workspace
	err := u.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		usr, err := users.VerifiedIn(ctx, tx, in.UserID)
		if err != nil {
			return err
		}
		if ws, err = u.createIn(ctx, tx, in.UserID, name); err != nil {
			return err
		}
		actor := events.Actor{Kind: events.ActorUser, ID: publicid.Encode(usr.ID), Name: usr.Name}
		return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceCreate, nil))
	})
	if err != nil {
		return View{}, err
	}
	return u.memberView(ctx, in.UserID, ws.PublicID, events.Actor{}, in.StrongSession)
}

// createIn creates the Workspace and its owner Membership on tx, with a fresh Workspace id and
// the Send cap a new Workspace starts with.
func (u *UseCase) createIn(ctx context.Context, tx *ent.Client, userID int64, name string) (*ent.Workspace, error) {
	sendCap, err := u.states.StartingSendCapIn(ctx, tx)
	if err != nil {
		return nil, err
	}
	ws, err := workspacesunchecked.CreateIn(ctx, tx, name, u.newPublicID, sendCap)
	if err != nil {
		return nil, err
	}
	if err := membershipsunchecked.AddCreatorIn(ctx, tx, userID, ws.ID); err != nil {
		return nil, err
	}
	return ws, nil
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
