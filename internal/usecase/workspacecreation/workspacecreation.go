// Package workspacecreation is the use case of creating a Workspace (ADR 0043, ADR 0102): a
// Verified User creates one and becomes its Owner. The Workspace with its Owner Membership and the
// Send cap it starts with (ADR 0067) are written through workspaces and memberships on one
// transaction. A User's first Workspace, created by the step that makes it Verified, is the Sign-up's
// (internal/usecase/signup), which composes the same two modules on that step's transaction.
//
// It takes the raw client's transactions: the Workspace belongs to no Workspace scope until it
// exists, so there is no *ent.Scoped to take.
package workspacecreation

import (
	"context"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase creates Workspaces.
type UseCase struct {
	bus *events.Bus
	// newPublicID draws a Workspace id.
	newPublicID func() string
	// states gives the Send cap a new Workspace starts with; nil gives none.
	states *instancestates.Module
}

// New builds the use case: the bus opens its transactions, states says the Send cap a Workspace
// starts with (instancestates.Module.StartingSendCapIn, ADR 0067; nil, as in a test, gives a new
// Workspace none), and newPublicID draws a Workspace id (workspaceid.New; a test gives one that
// collides to watch creation retry).
func New(bus *events.Bus, states *instancestates.Module, newPublicID func() string) *UseCase {
	return &UseCase{bus: bus, newPublicID: newPublicID, states: states}
}

// CreateWorkspace creates a Workspace named name for a Verified User, who becomes its
// Owner (ADR 0043). Names are free text and need not be unique. The Workspace, the Owner
// Membership and the `workspace.create` Audit entry are written atomically: a Workspace is
// reached through Memberships, so one without its owner row would be inaccessible. An
// Unverified User gets users.ErrEmailNotVerified.
// The name is trimmed; an empty one is workspaces.ErrNameEmpty, refused before the User is read.
func (u *UseCase) CreateWorkspace(ctx context.Context, userID int64, name string) (*ent.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, workspaces.ErrNameEmpty
	}
	for {
		var ws *ent.Workspace
		err := u.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			usr, err := tx.User.Get(ctx, userID)
			if err != nil {
				return err
			}
			if usr.EmailVerifiedAt == nil {
				return users.ErrEmailNotVerified
			}
			if ws, err = u.createIn(ctx, tx, userID, name); err != nil {
				return err
			}
			actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(usr.ID, 10), Name: usr.Name}
			return events.RecordAudit(ctx, pub, events.WorkspaceAudit(ws, actor, events.ActionWorkspaceCreate, nil))
		})
		if db.IsUniqueViolation(err) {
			continue // drew a taken id; the next attempt retries
		}
		return ws, err
	}
}

// createIn creates the Workspace and its owner Membership on tx, with a fresh Workspace id and
// the Send cap a new Workspace starts with. A unique violation (an id already
// taken) aborts tx, so the caller retries the whole transaction with a fresh id.
func (u *UseCase) createIn(ctx context.Context, tx *ent.Client, userID int64, name string) (*ent.Workspace, error) {
	var sendCap *int
	if u.states != nil {
		var err error
		if sendCap, err = u.states.StartingSendCapIn(ctx, tx); err != nil {
			return nil, err
		}
	}
	ws, err := workspaces.CreateIn(ctx, tx, name, u.newPublicID(), sendCap)
	if err != nil {
		return nil, err
	}
	if err := memberships.AddCreatorIn(ctx, tx, userID, ws.ID); err != nil {
		return nil, err
	}
	return ws, nil
}
