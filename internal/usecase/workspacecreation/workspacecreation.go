// Package workspacecreation is the use case of creating a Workspace (ADR 0043, ADR 0102): the
// Workspace with its Owner Membership and the Send cap it starts with (ADR 0067), written through
// workspaces and memberships on one transaction. A Verified User creates one, and every step that
// makes a User Verified creates the User's first one on the transaction of that step: the
// verification link, the reset link of an Unverified User, a Sign-up and an Owner made where
// verification is off, a provider sign-in that creates or takes over a User, and the completion
// of a pending sign-up. Those steps are this use case too: it opens their transaction and calls
// the users module's ...In functions for the User's part, so no identity module reads the Send
// caps. The Sign-up itself (ADR 0030, ADR 0031) is here whole, with verification required too,
// since its Verified path is one of those steps.
//
// It takes the raw client's transactions: a User and its first Workspace belong to no Workspace
// when the step starts, so there is no *ent.Scoped to take.
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
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/workspaceid"
)

// UseCase creates Workspaces, and makes Users Verified with their first one.
type UseCase struct {
	bus       *events.Bus
	users     *users.Module
	links     *users.Links
	passwords *credentials.Hasher
	// newPublicID draws a Workspace id (workspaceid.New by default).
	newPublicID func() string
	// sendCaps is whether the Instance has Send caps; only then does a new Workspace start with
	// the Instance's default Send cap.
	sendCaps providerlimit.SendCaps
	// signUp is what a Sign-up needs besides the users module (WithSignUp).
	signUp SignUpConfig
}

// Option tunes UseCase.
type Option func(*UseCase)

// WithWorkspaceIDs replaces the generator of Workspace ids (workspaceid.New by default). A test
// uses it to force a collision and watch creation retry.
func WithWorkspaceIDs(generate func() string) Option {
	return func(u *UseCase) { u.newPublicID = generate }
}

// WithSendCaps says whether the Instance has Send caps (ADR 0067); where it has, every Workspace
// created from now on starts with the default Send cap of the Instance state
// (instancestates.DefaultSendCapIn, read on the transaction that creates it). Without them (the
// default) a new Workspace has none, whatever default is stored.
func WithSendCaps(caps providerlimit.SendCaps) Option {
	return func(u *UseCase) { u.sendCaps = caps }
}

// New builds the use case: the bus opens its transactions, the users module writes the User's
// part of each step, links reads the verification and reset links, and passwords hashes the
// password of an Owner made from a shell.
func New(bus *events.Bus, usersModule *users.Module, links *users.Links, passwords *credentials.Hasher, opts ...Option) *UseCase {
	u := &UseCase{bus: bus, users: usersModule, links: links, passwords: passwords, newPublicID: workspaceid.New}
	for _, opt := range opts {
		opt(u)
	}
	return u
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
// the Send cap a new Workspace starts with (WithSendCaps). A unique violation (an id already
// taken) aborts tx, so the caller retries the whole transaction with a fresh id.
func (u *UseCase) createIn(ctx context.Context, tx *ent.Client, userID int64, name string) (*ent.Workspace, error) {
	var sendCap *int
	if u.sendCaps {
		var err error
		if sendCap, err = instancestates.DefaultSendCapIn(ctx, tx); err != nil {
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
