// Package accounts is the raw-client home for what the scoped client cannot
// express (ADR 0017): actors whose Workspace is not known up front. It owns the
// User (identity, credentials), the Membership lookup that turns a /w/{id} into
// a Workspace scope, the Workspace itself (it is the tenant, not a tenant-owned
// entity, so the scoped client has no wrapper for it), and the pending-Invitation
// lookup by token (the token is the only handle). The site handlers use it
// instead of holding a raw *ent.Client.
package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/workspaceid"
)

// Accounts reads and writes Users, Workspaces and Memberships over the raw client.
type Accounts struct {
	ent         *ent.Client
	bus         *events.Bus
	newPublicID func() string
}

// Option tunes Accounts.
type Option func(*Accounts)

// WithWorkspaceIDs replaces the generator of Workspace ids (workspaceid.New by default). A test
// uses it to force a collision and watch creation retry.
func WithWorkspaceIDs(generate func() string) Option {
	return func(a *Accounts) { a.newPublicID = generate }
}

// New builds Accounts over the ent client; the bus opens the transactions that
// span several writes.
func New(client *ent.Client, bus *events.Bus, opts ...Option) *Accounts {
	a := &Accounts{ent: client, bus: bus, newPublicID: workspaceid.New}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Scope resolves the Workspace addressed by a /w/{id} path segment for a User and
// returns its scoped client with the User's Membership (its User and Workspace
// loaded, for the Role and the Two-factor requirement). It is the site's construction
// point of the scoped client: access is "does this User have a Membership on this
// Workspace?". An ent NotFound error means the Workspace id does not exist or the User is
// not a member.
func (a *Accounts) Scope(ctx context.Context, userID int64, publicID string) (*ent.Scoped, *ent.Membership, error) {
	m, err := a.ent.Membership.Query().
		Where(membership.UserID(userID), membership.HasWorkspaceWith(workspace.PublicID(publicID), Live())).
		WithUser().
		WithWorkspace().
		Only(ctx)
	if err != nil {
		return nil, nil, err
	}
	return a.ScopeOf(m), m, nil
}

// ScopeOf is the scoped client of a loaded Membership's Workspace, acting as its User
// (the User edge, when loaded, names the actor). It is Scope without the lookup, for a
// caller that already holds the Membership.
func (a *Accounts) ScopeOf(m *ent.Membership) *ent.Scoped {
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(m.UserID, 10)}
	if u := m.Edges.User; u != nil {
		actor.Name = u.Name
	}
	return a.bus.Act(a.ent.Scoped(m.WorkspaceID), actor)
}

// BootstrapScope is the scoped client of the oldest Workspace, for the bootstrap
// token (no caller token exists yet to say which Workspace is meant).
func (a *Accounts) BootstrapScope(ctx context.Context) (*ent.Scoped, error) {
	id, err := a.ent.Workspace.Query().Where(Live()).Order(ent.Asc(workspace.FieldID)).FirstID(ctx)
	if err != nil {
		return nil, err
	}
	return events.Ingest(a.ent.Scoped(id)), nil
}

// MembershipsOf lists a User's Memberships with their User and Workspace loaded, oldest
// Workspace first.
func (a *Accounts) MembershipsOf(ctx context.Context, userID int64) ([]*ent.Membership, error) {
	return a.ent.Membership.Query().
		Where(membership.UserID(userID), membership.HasWorkspaceWith(Live())).
		WithUser().
		WithWorkspace().
		Order(ent.Asc(membership.FieldWorkspaceID)).
		All(ctx)
}

// UpdateWorkspace renames the scoped Workspace and, when postalAddress is non-nil,
// sets (or clears, with "") its postal address. A change is recorded as a
// `workspace.update` Audit entry in the same transaction (ADR 0022): the Workspace is
// the tenant root and has no scoped wrapper, so this is an explicit path.
func (a *Accounts) UpdateWorkspace(ctx context.Context, s *ent.Scoped, actor events.Actor, name string, postalAddress *string) (*ent.Workspace, error) {
	var updated *ent.Workspace
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID()).SetName(name)
		if postalAddress != nil {
			upd = upd.SetPostalAddress(*postalAddress)
		}
		updated, err = upd.Save(ctx)
		if err != nil {
			return err
		}
		diff := map[string]any{}
		if before.Name != updated.Name {
			diff["name"] = map[string]any{"from": before.Name, "to": updated.Name}
		}
		if before.PostalAddress != updated.PostalAddress {
			diff["postal_address"] = map[string]any{"from": before.PostalAddress, "to": updated.PostalAddress}
		}
		if len(diff) == 0 {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(updated.ID, 10),
			TargetName:  updated.Name,
			Diff:        diff,
		})
	})
	return updated, err
}

// SetAuditRetention sets (or, with nil, clears) the number of days the Audit log is
// kept (the EE advanced-retention window, ADR 0014) and records the change as a
// `workspace.update` Audit entry in the same transaction. The caller has checked the
// license and the role.
func (a *Accounts) SetAuditRetention(ctx context.Context, s *ent.Scoped, actor events.Actor, days *int) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if days == nil {
			upd = upd.ClearRetentionDays()
		} else {
			upd = upd.SetRetentionDays(*days)
		}
		if _, err := upd.Save(ctx); err != nil {
			return err
		}
		if ptrEqual(before.RetentionDays, days) {
			return nil
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff:        map[string]any{"retention_days": map[string]any{"from": before.RetentionDays, "to": days}},
		})
	})
}

// SetSecondFactorRequirement switches the scoped Workspace's Two-factor requirement
// (ADR 0020) on, starting at now, or off. Switching it on while it is on keeps the
// original start, so no member's grace restarts. A change is recorded as a
// `workspace.update` Audit entry in the same transaction. The caller has checked the
// role. switchedOn reports that this call turned the requirement on.
func (a *Accounts) SetSecondFactorRequirement(ctx context.Context, s *ent.Scoped, actor events.Actor, required bool, now time.Time) (updated *ent.Workspace, switchedOn bool, err error) {
	err = a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		before, err := tx.Workspace.Get(ctx, s.WorkspaceID())
		if err != nil {
			return err
		}
		updated = before
		if (before.SecondFactorRequiredAt != nil) == required {
			return nil
		}
		upd := tx.Workspace.UpdateOneID(s.WorkspaceID())
		if required {
			upd = upd.SetSecondFactorRequiredAt(now)
		} else {
			upd = upd.ClearSecondFactorRequiredAt()
		}
		if updated, err = upd.Save(ctx); err != nil {
			return err
		}
		switchedOn = required
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionWorkspaceUpdate,
			TargetType:  workspace.Label,
			TargetID:    strconv.FormatInt(before.ID, 10),
			TargetName:  before.Name,
			Diff: map[string]any{"second_factor_required_at": map[string]any{
				"from": before.SecondFactorRequiredAt, "to": updated.SecondFactorRequiredAt,
			}},
		})
	})
	return updated, switchedOn, err
}

func ptrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ErrEmailNotVerified refuses creating a Workspace for a User whose email is not proven
// (ADR 0030): sign-up verification must not be bypassed by creating Workspaces first.
var ErrEmailNotVerified = errors.New("accounts: the email is not verified")

// CreateWorkspace creates a Workspace named name for a Verified User, who becomes its
// Owner (ADR 0043). Names are free text and need not be unique. The Workspace, the Owner
// Membership and the `workspace.create` Audit entry are written atomically: a Workspace is
// reached through Memberships, so one without its owner row would be inaccessible. An
// Unverified User gets ErrEmailNotVerified.
func (a *Accounts) CreateWorkspace(ctx context.Context, userID int64, name string) (*ent.Workspace, error) {
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
			if ws, err = createWorkspaceIn(ctx, tx, userID, name, a.newPublicID()); err != nil {
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
// Workspace id it is given. A unique violation (an id already taken) aborts tx, so the caller retries the whole
// transaction with a fresh id.
func createWorkspaceIn(ctx context.Context, tx *ent.Client, userID int64, name, publicID string) (*ent.Workspace, error) {
	collectKey, err := credentials.GenerateCollectKey()
	if err != nil {
		return nil, err
	}
	ws, err := tx.Workspace.Create().
		SetName(name).
		SetPublicID(publicID).
		SetCollectKey(collectKey).
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

// User loads a User by id.
func (a *Accounts) User(ctx context.Context, id int64) (*ent.User, error) {
	return a.ent.User.Get(ctx, id)
}

// UserByEmail loads a User by login email (an ent NotFound error when none).
func (a *Accounts) UserByEmail(ctx context.Context, email string) (*ent.User, error) {
	return a.ent.User.Query().Where(user.Email(email)).Only(ctx)
}

// UserByLoginAddress loads a User by a normalized login address, ignoring case (an ent
// NotFound error when none): the sign-in address is lower-cased once by the login module,
// while a stored email keeps the case it was registered with.
func (a *Accounts) UserByLoginAddress(ctx context.Context, address string) (*ent.User, error) {
	return a.ent.User.Query().Where(user.EmailEqualFold(address)).Order(ent.Asc(user.FieldID)).First(ctx)
}

// EmailTaken reports whether an account already uses the login email.
func (a *Accounts) EmailTaken(ctx context.Context, email string) (bool, error) {
	return a.ent.User.Query().Where(user.Email(email)).Exist(ctx)
}

// Register creates an Unverified User (ADR 0030): a name, an email and the password hash
// from the registration form, no Membership. The password goes live only when the address
// is verified from the Registration browser, or replaced on the link. The unique index on the email refuses a taken address (see
// db.IsUniqueViolation).
func (a *Accounts) Register(ctx context.Context, name, email, passwordHash string) (*ent.User, error) {
	return a.ent.User.Create().SetName(name).SetEmail(email).SetPasswordHash(passwordHash).Save(ctx)
}

// ReplaceUnverifiedPassword is a repeated registration of an address (ADR 0030): it
// stores the new password hash on the User while it is still Unverified, which ends
// every verification link minted over the earlier hash. A Verified User is left
// untouched. It returns the User as it stands afterwards.
func (a *Accounts) ReplaceUnverifiedPassword(ctx context.Context, id int64, passwordHash string) (*ent.User, error) {
	_, err := a.ent.User.Update().
		Where(user.ID(id), user.EmailVerifiedAtIsNil()).
		SetPasswordHash(passwordHash).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return a.ent.User.Get(ctx, id)
}

// CreateUser creates a User whose email is already proven, with a password hash: a
// User made by the platform itself rather than by a registration.
func (a *Accounts) CreateUser(ctx context.Context, name, email, passwordHash string) (*ent.User, error) {
	return a.ent.User.Create().SetName(name).SetEmail(email).SetPasswordHash(passwordHash).SetEmailVerifiedAt(time.Now()).Save(ctx)
}

// markVerified completes an Unverified User's row: it stamps the Verified email, bumps the
// session epoch (ending every earlier session), sets the password hash when passwordHash is
// not nil (nil keeps the stored one) and the name when not empty. The write lands only on a still-Unverified User, so of two
// racing completions exactly one wins; the other gets ErrAlreadyVerified and changes
// nothing.
func markVerified(ctx context.Context, tx *ent.Client, id int64, name string, passwordHash *string) (*ent.User, error) {
	upd := tx.User.Update().
		Where(user.ID(id), user.EmailVerifiedAtIsNil()).
		SetEmailVerifiedAt(time.Now()).
		SetEmailProvenAt(time.Now()).
		AddSessionEpoch(1)
	if passwordHash != nil {
		upd = upd.SetPasswordHash(*passwordHash)
	}
	if name != "" {
		upd = upd.SetName(name)
	}
	n, err := upd.Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrAlreadyVerified
	}
	return tx.User.Get(ctx, id)
}

// FirstWorkspaceName is the fixed name of the Workspace made for a User at verification.
const FirstWorkspaceName = "Personal"

// VerifyEmail completes a registration from its link (ADR 0030), in one transaction:
// it stamps the Verified email, sets the password hash to *passwordHash (nil keeps the one
// stored at registration), bumps the session epoch (ending every session issued before, as
// a password change does) and creates the User's first Workspace, named FirstWorkspaceName,
// with the User as owner. It records `user.email_verify` in that Workspace's Audit log and
// returns the User.
// ErrAlreadyVerified when the User is no longer Unverified (a used link).
func (a *Accounts) VerifyEmail(ctx context.Context, id int64, passwordHash *string) (*ent.User, error) {
	return a.verified(ctx, func(tx *ent.Client) (*ent.User, error) {
		return markVerified(ctx, tx, id, "", passwordHash)
	})
}

// RegisterVerified is registration on an install that does not require email
// verification (ADR 0030): in one transaction it creates a User that is Verified at
// once with its password, and the first Workspace (FirstWorkspaceName), exactly as VerifyEmail does. The
// unique index on the email refuses a taken address (see db.IsUniqueViolation).
func (a *Accounts) RegisterVerified(ctx context.Context, name, email, passwordHash string) (*ent.User, error) {
	return a.verified(ctx, func(tx *ent.Client) (*ent.User, error) {
		return tx.User.Create().
			SetName(name).
			SetEmail(email).
			SetPasswordHash(passwordHash).
			SetEmailVerifiedAt(time.Now()).
			Save(ctx)
	})
}

// CreateOwner is the Instance operator's way to make a first Owner on an Instance that
// closed sign-up (ADR 0042): a Verified User with a generated password and its first
// Workspace, named FirstWorkspaceName, exactly as RegisterVerified makes them. The password
// is returned once; the unique index on the email refuses a taken address (see
// db.IsUniqueViolation).
func (a *Accounts) CreateOwner(ctx context.Context, email, name string) (password string, err error) {
	password = rand.Text()
	hash, err := credentials.HashPassword(password)
	if err != nil {
		return "", err
	}
	if _, err := a.RegisterVerified(ctx, name, email, hash); err != nil {
		return "", err
	}
	return password, nil
}

// verified is what makes a User Verified, shared by VerifyEmail and RegisterVerified:
// write stores the User's verified state, then the User's first Workspace is created
// and `user.email_verify` recorded in its Audit log, all in one transaction.
func (a *Accounts) verified(ctx context.Context, write func(tx *ent.Client) (*ent.User, error)) (*ent.User, error) {
	for {
		var verified *ent.User
		written := false
		err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
			u, err := write(tx)
			if err != nil {
				return err
			}
			written = true
			if _, err := createWorkspaceIn(ctx, tx, u.ID, FirstWorkspaceName, a.newPublicID()); err != nil {
				return err
			}
			verified = u
			return RecordUserAction(ctx, tx, pub, u, events.ActionUserEmailVerify, nil)
		})
		if written && db.IsUniqueViolation(err) {
			continue // drew a taken id; the next attempt retries
		}
		return verified, err
	}
}

// UpdateProfile sets the User's name and/or password hash (nil leaves a field
// unchanged) and returns the stored User. A password change bumps the session
// epoch, ending every session issued before it (ADR 0020), and is recorded as an
// Audit entry in every Workspace the User belongs to, in the same transaction.
func (a *Accounts) UpdateProfile(ctx context.Context, id int64, name, passwordHash *string) (*ent.User, error) {
	return a.updateProfile(ctx, id, name, passwordHash, false)
}

// updateProfile is UpdateProfile; proven also stamps the address as proven (a reset link
// was followed, which was mailed to it).
func (a *Accounts) updateProfile(ctx context.Context, id int64, name, passwordHash *string, proven bool) (*ent.User, error) {
	var saved *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		upd := tx.User.UpdateOneID(id)
		if proven {
			upd = upd.SetEmailProvenAt(time.Now())
		}
		if name != nil {
			upd = upd.SetName(*name)
		}
		if passwordHash != nil {
			// A new password ends every session issued before it (ADR 0020).
			upd = upd.SetPasswordHash(*passwordHash).AddSessionEpoch(1)
		}
		u, err := upd.Save(ctx)
		if err != nil {
			return err
		}
		saved = u
		if passwordHash == nil {
			return nil
		}
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserPasswordChange, map[string]any{"password": "changed"})
	})
	return saved, err
}

// ResetPassword sets the User's password from a reset link (ADR 0030). The link was
// mailed to the login address, so following it proves the address as much as the
// verification link does: for an Unverified User it completes the registration
// exactly as VerifyEmail (Verified email, first Workspace, `user.email_verify`) and
// reports completedRegistration = true with the User, so the caller sends the welcome
// email. For a Verified User it is only a password change, recorded like UpdateProfile.
// Either way every earlier session ends.
func (a *Accounts) ResetPassword(ctx context.Context, id int64, passwordHash string) (u *ent.User, completedRegistration bool, err error) {
	u, err = a.User(ctx, id)
	if err != nil {
		return nil, false, err
	}
	if u.EmailVerifiedAt == nil {
		u, err = a.VerifyEmail(ctx, id, &passwordHash)
		if err == nil {
			return u, true, nil
		}
		if !errors.Is(err, ErrAlreadyVerified) {
			return nil, false, err
		}
		// A concurrent link verified the User first: this is now a plain password change.
	}
	u, err = a.updateProfile(ctx, id, nil, &passwordHash, true)
	return u, false, err
}

// EndSessions bumps the User's session epoch, ending every session issued before
// it on every device (ADR 0020, "sign out everywhere"), and records
// `user.sign_out_everywhere` in the same transaction.
func (a *Accounts) EndSessions(ctx context.Context, id int64) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.UpdateOneID(id).AddSessionEpoch(1).Save(ctx)
		if err != nil {
			return err
		}
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserSignOutEverywhere, nil)
	})
}

// LoginPassword is the method of a login by email and password in the `user.login` entry.
const LoginPassword = "password"

// RecordLogin records a successful sign-in of the User as `user.login` in the log of
// every Workspace they belong to (ADR 0022: a login belongs to a User, not a
// Workspace, so there is no account-level log). The entry names the method: LoginPassword
// or the registry name of the Sign-in provider (ADR 0048).
func (a *Accounts) RecordLogin(ctx context.Context, u *ent.User, method string) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		return RecordUserAction(ctx, tx, pub, u, events.ActionUserLogin, map[string]any{"method": method})
	})
}

// RecordUserAction publishes one Audit entry per Workspace the User holds a
// Membership in, and none anywhere else. The User is the actor and the target.
func RecordUserAction(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User, action string, diff map[string]any) error {
	actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(u.ID, 10), Name: u.Name}
	return RecordActionOnUser(ctx, tx, pub, actor, u, action, diff)
}

// RecordActionOnUser publishes action by actor on the User u into the log of every
// Workspace u holds a Membership in (a staff member acting on a User), and none
// anywhere else.
func RecordActionOnUser(ctx context.Context, tx *ent.Client, pub events.Publisher, actor events.Actor, u *ent.User, action string, diff map[string]any) error {
	memberships, err := tx.Membership.Query().Where(membership.UserID(u.ID)).All(ctx)
	if err != nil {
		return err
	}
	ids := make([]int64, len(memberships))
	for i, m := range memberships {
		ids[i] = m.WorkspaceID
	}
	return RecordActionOnUserIn(ctx, pub, actor, u, action, diff, ids...)
}

// RecordActionOnUserIn publishes action by actor on the User u into the logs of the
// given Workspaces only: an Owner or Admin acting on a member is recorded where they
// hold that authority, not in the member's other Workspaces.
func RecordActionOnUserIn(ctx context.Context, pub events.Publisher, actor events.Actor, u *ent.User, action string, diff map[string]any, workspaceIDs ...int64) error {
	id := strconv.FormatInt(u.ID, 10)
	for _, ws := range workspaceIDs {
		if err := events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: ws,
			Actor:       actor,
			Action:      action,
			TargetType:  user.Label,
			TargetID:    id,
			TargetName:  u.Name,
			Diff:        diff,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ChangeEmail swaps the login email for one proven by a confirmation link, so it is
// stored verified, and bumps the session epoch: every session issued before the
// change ends (ADR 0020). The unique index guards a race on the address (see
// db.IsUniqueViolation).
func (a *Accounts) ChangeEmail(ctx context.Context, id int64, newEmail string) error {
	return a.bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		now := time.Now()
		return tx.User.UpdateOneID(id).SetEmail(newEmail).SetEmailVerifiedAt(now).SetEmailProvenAt(now).AddSessionEpoch(1).Exec(ctx)
	})
}

// PendingInvitation looks up a pending, unexpired Invitation by its raw token, with
// its Workspace edge. The token is the authorization, so no Workspace is known up
// front. An ent NotFound error when there is none.
func (a *Accounts) PendingInvitation(ctx context.Context, token string) (*ent.Invitation, error) {
	return a.ent.Invitation.Query().
		Where(
			invitation.TokenHash(HashInviteToken(token)),
			invitation.AcceptedAtIsNil(),
			invitation.ExpiresAtGT(time.Now()),
		).
		WithWorkspace().
		Only(ctx)
}

// AcceptInvitation turns a pending Invitation into a Membership. sessionEmail is the
// login email of the session the request carried ("" for none). A Verified User is
// attached only with their own session (ErrSignInRequired without one, ErrWrongAccount
// for another User's), decided on the row read inside the transaction. For an address with
// no User or an Unverified User (ADR 0030) the name and password are required
// (ErrCredentialsRequired otherwise): the link proves control of the address, so the
// User is created, or the Unverified User's row completed (name and password set,
// Verified email stamped, session epoch bumped, `user.email_verify` audited), and
// nothing the registrant set survives; if the address was verified meanwhile that is ErrAlreadyVerified. A
// Verified User is attached as is. The Membership is created (a
// duplicate is not an error), the Invitation marked accepted and the acceptance
// audited, all in one transaction.
func (a *Accounts) AcceptInvitation(ctx context.Context, inv *ent.Invitation, name, password, sessionEmail string) error {
	role := membership.Role(inv.Role)
	return a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, uerr := tx.User.Query().Where(user.EmailEqualFold(inv.Email)).Only(ctx)
		if uerr != nil && !ent.IsNotFound(uerr) {
			return uerr
		}
		provedByInvitation := false
		if uerr == nil && u.EmailVerifiedAt != nil {
			// A Verified User's own session is the proof of the address (ADR 0030).
			if sessionEmail == "" {
				return ErrSignInRequired
			}
			if !strings.EqualFold(sessionEmail, inv.Email) {
				return ErrWrongAccount
			}
			// The invitation reached the address, and its owner's session accepted it.
			if err := tx.User.Update().Where(user.ID(u.ID), user.EmailProvenAtIsNil()).SetEmailProvenAt(time.Now()).Exec(ctx); err != nil {
				return err
			}
		} else {
			name = strings.TrimSpace(name)
			if name == "" || password == "" {
				return ErrCredentialsRequired
			}
			hash, herr := credentials.HashPassword(password)
			if herr != nil {
				return herr
			}
			if ent.IsNotFound(uerr) {
				u, uerr = tx.User.Create().
					SetName(name).
					SetEmail(inv.Email).
					SetPasswordHash(hash).
					SetEmailVerifiedAt(time.Now()).
					SetEmailProvenAt(time.Now()).
					Save(ctx)
			} else {
				u, uerr = markVerified(ctx, tx, u.ID, name, &hash)
			}
			if uerr != nil {
				return uerr
			}
			provedByInvitation = true
		}

		ts := tx.Scoped(inv.WorkspaceID)
		actor := events.Actor{Kind: events.ActorUser, ID: strconv.FormatInt(u.ID, 10), Name: u.Name}
		existing, merr := ts.Membership().Query().Where(membership.UserID(u.ID)).Only(ctx)
		if merr != nil && !ent.IsNotFound(merr) {
			return merr
		}
		if ent.IsNotFound(merr) {
			if _, cerr := ts.Membership().Create().
				SetUserID(u.ID).
				SetRole(role).
				SetSource(membership.SourceInvitation).
				Save(ctx); cerr != nil {
				return cerr
			}
		} else if existing.Role == membership.RoleMember && role == membership.RoleAdmin {
			// An explicit Invitation raises a member to admin; it never lowers a Role.
			if err := ts.Membership().UpdateOneID(existing.ID).SetRole(membership.RoleAdmin).Exec(ctx); err != nil {
				return err
			}
			if err := events.RecordAudit(ctx, pub, &events.AuditEntry{
				WorkspaceID: inv.WorkspaceID,
				Actor:       actor,
				Action:      events.ActionMembershipUpdate,
				TargetType:  membership.Label,
				TargetID:    strconv.FormatInt(existing.ID, 10),
				TargetName:  u.Email,
				Diff:        map[string]any{"role": map[string]any{"from": string(existing.Role), "to": string(role)}},
			}); err != nil {
				return err
			}
		}

		if err := ts.Invitation().UpdateOneID(inv.ID).SetAcceptedAt(time.Now()).Exec(ctx); err != nil {
			return err
		}
		if provedByInvitation {
			if err := RecordActionOnUserIn(ctx, pub, actor, u, events.ActionUserEmailVerify, nil, inv.WorkspaceID); err != nil {
				return err
			}
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: inv.WorkspaceID,
			Actor:       actor,
			Action:      events.ActionInvitationAccept,
			TargetType:  invitation.Label,
			TargetID:    strconv.FormatInt(inv.ID, 10),
			TargetName:  inv.Email,
			Diff:        map[string]any{"role": map[string]any{"to": string(inv.Role)}},
		})
	})
}

// InviteInput is a new (or reissued) Invitation request. The token, its hash and
// the expiry are the Invite operation's to decide, not the caller's.
type InviteInput struct {
	Email     string
	Role      invitation.Role
	InvitedBy int64
}

// Invite creates the Invitation for an email, or reissues the pending one (token,
// expiry, role; a prior acceptance is cleared), and records `invitation.create` in
// the same transaction. It generates the one-time token, stores only its hash, sets
// the expiry InviteTokenTTL ahead and returns the raw token for the caller's link;
// the token itself is never logged. ErrEmailEmpty for a blank email, ErrAlreadyMember
// when the address already has a Membership in the Workspace. Delivery is the
// caller's concern.
func (a *Accounts) Invite(ctx context.Context, s *ent.Scoped, actor events.Actor, in InviteInput) (*ent.Invitation, string, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return nil, "", ErrEmailEmpty
	}
	token, err := generateInviteToken()
	if err != nil {
		return nil, "", err
	}
	tokenHash := HashInviteToken(token)
	expiresAt := time.Now().Add(InviteTokenTTL)

	var inv *ent.Invitation
	err = a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		member, err := ts.Membership().Query().
			Where(membership.HasUserWith(user.Email(email))).
			Exist(ctx)
		if err != nil {
			return err
		}
		if member {
			return ErrAlreadyMember
		}
		existing, err := ts.Invitation().Query().Where(invitation.Email(email)).Only(ctx)
		switch {
		case ent.IsNotFound(err):
			inv, err = ts.Invitation().Create().
				SetEmail(email).SetRole(in.Role).SetTokenHash(tokenHash).
				SetExpiresAt(expiresAt).SetInvitedBy(in.InvitedBy).
				Save(ctx)
		case err != nil:
		default:
			inv, err = ts.Invitation().UpdateOneID(existing.ID).
				SetRole(in.Role).SetTokenHash(tokenHash).
				SetExpiresAt(expiresAt).SetInvitedBy(in.InvitedBy).
				ClearAcceptedAt().
				Save(ctx)
		}
		if err != nil {
			return err
		}
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionInvitationCreate,
			TargetType:  invitation.Label,
			TargetID:    strconv.FormatInt(inv.ID, 10),
			TargetName:  inv.Email,
			Diff:        map[string]any{"role": map[string]any{"to": string(inv.Role)}},
		})
	})
	if err != nil {
		return nil, "", err
	}
	return inv, token, nil
}

// RevokeInvitation deletes a pending Invitation and records `invitation.revoke`. It
// reports whether the Invitation existed in the scoped Workspace.
func (a *Accounts) RevokeInvitation(ctx context.Context, s *ent.Scoped, actor events.Actor, id int64) (bool, error) {
	found := false
	err := a.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		inv, err := ts.Invitation().Query().Where(invitation.ID(id)).Only(ctx)
		if ent.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ts.Invitation().DeleteOneID(id).Exec(ctx); err != nil {
			return err
		}
		found = true
		return events.RecordAudit(ctx, pub, &events.AuditEntry{
			WorkspaceID: s.WorkspaceID(),
			Actor:       actor,
			Action:      events.ActionInvitationRevoke,
			TargetType:  invitation.Label,
			TargetID:    strconv.FormatInt(id, 10),
			TargetName:  inv.Email,
		})
	})
	return found, err
}

// WorkspaceIDByPublicID resolves a Workspace id (ADR 0043) to the internal key; command-line
// tooling addresses Workspaces by their id.
func WorkspaceIDByPublicID(ctx context.Context, client *ent.Client, publicID string) (int64, error) {
	id, err := client.Workspace.Query().Where(workspace.PublicID(publicID)).OnlyID(ctx)
	if err != nil {
		return 0, fmt.Errorf("workspace %q: %w", publicID, err)
	}
	return id, nil
}

// Live is a Workspace that is not deleted (ADR 0043): every lookup that must not reach a
// deleted Workspace (its Members' scope, the lists, the Instance-wide readers) uses it, so
// "live" is stated once.
func Live() predicate.Workspace { return workspace.DiscardedAtIsNil() }
