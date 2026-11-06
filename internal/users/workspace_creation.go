package users

import (
	"context"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/workspaces"
)

// Creating a Workspace with its Owner Membership and the Send cap it starts with (ADR 0043, ADR
// 0067) spans the Workspace, the Membership and the send limits, so it is the use case
// workspacecreation of ADR 0102 to come. Until then it lives here, beside the User whose
// verification creates the first Workspace (verified, in users.go) on the same transaction.

// DefaultSendCap reads, on the transaction that creates a Workspace, the Send cap it starts with
// (ADR 0067): nil for none. instance.DefaultSendCapIn is the Instance's default.
type DefaultSendCap func(ctx context.Context, tx *ent.Client) (*int, error)

// WithWorkspaceIDs replaces the generator of Workspace ids (workspaceid.New by default). A test
// uses it to force a collision and watch creation retry.
func WithWorkspaceIDs(generate func() string) Option {
	return func(a *Module) { a.newPublicID = generate }
}

// WithSendCaps says whether the Instance has Send caps (ADR 0067); where it has, every Workspace
// created from now on starts with the default Send cap defaultIn reads. Without them (the
// default) a new Workspace has none, whatever default is stored.
func WithSendCaps(caps providerlimit.SendCaps, defaultIn DefaultSendCap) Option {
	return func(a *Module) { a.sendCaps, a.defaultSendCap = caps, defaultIn }
}

// CreateWorkspace creates a Workspace named name for a Verified User, who becomes its
// Owner (ADR 0043). Names are free text and need not be unique. The Workspace, the Owner
// Membership and the `workspace.create` Audit entry are written atomically: a Workspace is
// reached through Memberships, so one without its owner row would be inaccessible. An
// Unverified User gets ErrEmailNotVerified.
// The name is trimmed; an empty one is workspaces.ErrNameEmpty, refused before the User is read.
func (a *Module) CreateWorkspace(ctx context.Context, userID int64, name string) (*ent.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, workspaces.ErrNameEmpty
	}
	for {
		var ws *ent.Workspace
		err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			u, err := tx.User.Get(ctx, userID)
			if err != nil {
				return err
			}
			if u.EmailVerifiedAt == nil {
				return ErrEmailNotVerified
			}
			if ws, err = a.createWorkspaceIn(ctx, tx, userID, name, a.newPublicID()); err != nil {
				return err
			}
			actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(u.ID, 10), Name: u.Name}
			return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceCreate, nil))
		})
		if db.IsUniqueViolation(err) {
			continue // drew a taken id; the next attempt retries
		}
		return ws, err
	}
}

// createWorkspaceIn creates the Workspace and its owner Membership on tx, with the
// Workspace id it is given and the Send cap a new Workspace starts with (WithSendCaps). A unique violation (an id already taken) aborts tx, so the caller retries the whole
// transaction with a fresh id.
func (a *Module) createWorkspaceIn(ctx context.Context, tx *ent.Client, userID int64, name, publicID string) (*ent.Workspace, error) {
	collectKey, err := credentials.GenerateCollectKey()
	if err != nil {
		return nil, err
	}
	var sendCap *int
	if a.sendCaps {
		if sendCap, err = a.defaultSendCap(ctx, tx); err != nil {
			return nil, err
		}
	}
	ws, err := tx.Workspace.Create().
		SetName(name).
		SetPublicID(publicID).
		SetCollectKey(collectKey).
		SetNillableSendCap(sendCap).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Membership.Create().
		SetUserID(userID).
		SetWorkspaceID(ws.ID).
		SetRole(membership.RoleOwner).
		SetSource(membership.SourceCreator).
		Save(ctx); err != nil {
		return nil, err
	}
	return ws, nil
}
