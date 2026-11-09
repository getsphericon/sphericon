// Package userdeletion is the use case of User deletion (GLOSSARY, ADR 0050, ADR 0110, ADR 0111):
// a User's own immediate and irreversible removal of themself. It spans the User
// (internal/domain/identity/users), the Login throttle (internal/domain/identity/login), the
// Second factor (internal/domain/identity/secondfactors, which sits above users) and the
// Memberships and Workspaces the User leaves, so no single module owns it.
//
// The User belongs to no Workspace, so there is no *ent.Scoped to take: this use case holds the
// raw *ent.Client of its transaction (the User is reached by id), one of the closed list of
// docs/architecture/workspace-scoping.md (Workspace scoping).
package userdeletion

import (
	"context"
	"errors"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/login"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships/membershipsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/users/usersunchecked"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// Request is a User's ask to delete themself.
type Request struct {
	// UserID is the signed-in User who asks.
	UserID int64
	// Email is the address the User typed to confirm; it must be their own.
	Email string
	// Password proves a User who has one.
	Password string
	// Token is the mailed deletion link that proves a User with no password.
	Token string
	// Code is the Second factor code (a TOTP code or a Recovery code) of a User who has one.
	Code string
}

// Outcome is what a deletion decided.
type Outcome int

const (
	// Throttled: the address's Login throttle delay is running. Result.Wait, Limit and Now say
	// how long.
	Throttled Outcome = iota + 1
	// Deleted: the User is gone.
	Deleted
)

// Result is the one answer of Delete.
type Result struct {
	// Delay is set for Throttled.
	login.Delay

	Outcome Outcome
}

// UseCase deletes Users.
type UseCase struct {
	bus     *events.Bus
	users   *users.Module
	links   *users.Links
	factors *secondfactors.Module
	login   *login.Module[*ent.User]
}

// New builds the use case over the bus whose transaction it opens. budget is the Login throttle
// of the Users (throttle.KindLogin), the budget every proof of a signed-in User feeds, and passwords
// the hasher the stored passwords were made with: the password is proven through the login
// module's one prove path over them.
func New(bus *events.Bus, usersModule *users.Module, links *users.Links, factors *secondfactors.Module, budget login.Throttle, passwords *credentials.Hasher) *UseCase {
	return &UseCase{
		bus: bus, users: usersModule, links: links, factors: factors,
		login: login.New(login.Config[*ent.User]{Throttle: budget, Passwords: passwords}),
	}
}

// Delete erases the User who asks at once and for good, after their proof:
//
//   - the typed email must be theirs (users.ErrDeletionEmailMismatch);
//   - a User with a password proves it through the login module's prove path (a wrong one is
//     login.ErrPasswordIncorrect and counted on the Login throttle); a User with none proves the
//     mailbox with the deletion link (users.ErrConfirmLinkInvalid), after the Login throttle is
//     consulted, since nothing in that proof feeds it (ADR 0025). While the delay runs the
//     answer is Throttled and nothing is checked;
//   - a User with a Second factor confirms with a code, spent inside the deletion's transaction
//     so only if it commits; a wrong one is secondfactors.ErrDeletionCodeInvalid, counted like a
//     wrong password.
//
// The transaction locks the User row, then the Owner rows of every Workspace the User belongs
// to, in ascending Workspace id order, so two co-Owners deleting at once serialize on the
// Workspace and cannot deadlock: the later one reads the earlier one's outcome. Only then does it
// read the Memberships and Roles the decision rests on. It refuses with users.ErrSoleOwner
// (naming the Workspaces) while the User is the only Owner of a Workspace that is not in its
// deletion window (ADR 0043); a sole-owned Workspace in the window does not block and is erased on
// schedule. The proof is re-checked on the locked row: a password set or removed since it was
// read is users.ErrProofStale. Every Membership then goes through its Workspace's scope with the
// User as actor of a `membership.delete` Audit entry, and the User row is deleted, which publishes
// events.UserDeleted.
func (uc *UseCase) Delete(ctx context.Context, req Request) (Result, error) {
	u, err := uc.users.DeletionSubject(ctx, req.UserID, req.Email)
	if err != nil {
		return Result{}, err
	}
	hadPassword := u.PasswordHash != ""
	if hadPassword {
		proof, err := uc.login.Prove(ctx, u.Email, u.PasswordHash, req.Password)
		if err != nil {
			return Result{}, err
		}
		if proof.Proof == login.ProofThrottled {
			return Result{Outcome: Throttled, Delay: proof.Delay}, nil
		}
	} else {
		delay, throttled, err := uc.login.Throttled(ctx, u.Email)
		if err != nil {
			return Result{}, err
		}
		if throttled {
			return Result{Outcome: Throttled, Delay: delay}, nil
		}
		if err := uc.links.CheckDeletionLink(req.Token, u); err != nil {
			return Result{}, err
		}
	}

	err = uc.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		return uc.deleteIn(ctx, tx, pub, u.ID, hadPassword, req.Code)
	})
	if errors.Is(err, secondfactors.ErrDeletionCodeInvalid) {
		// The wrong code is counted like a wrong password (ADR 0025), after the rollback.
		if cerr := uc.login.CountFailure(ctx, u.Email); cerr != nil {
			return Result{}, cerr
		}
	}
	if err != nil {
		return Result{}, err
	}
	return Result{Outcome: Deleted}, nil
}

// deleteIn is the deletion's transaction tx, for the User userID whose proof read hadPassword.
func (uc *UseCase) deleteIn(ctx context.Context, tx *ent.Client, pub events.Publisher, userID int64, hadPassword bool, code string) error {
	u, err := users.LockIn(ctx, tx, userID)
	if err != nil {
		return err
	}
	held, soleOwned, err := memberships.SoleOwnershipIn(ctx, tx, userID)
	if err != nil {
		return err
	}
	live, err := workspaces.LiveIn(ctx, tx, soleOwned...)
	if err != nil {
		return err
	}
	if len(live) > 0 {
		return &users.DeletionRefusedError{Reason: users.ErrSoleOwner, Workspaces: live}
	}
	if (u.PasswordHash != "") != hadPassword {
		return &users.DeletionRefusedError{Reason: users.ErrProofStale}
	}
	if err := uc.factors.DeletionCode(ctx, tx, pub, u, code); err != nil {
		return err
	}
	for _, m := range held {
		ts := tx.Scoped(m.WorkspaceID)
		// Only a sole-owned Workspace still standing here is in its deletion window.
		if slices.Contains(soleOwned, m.WorkspaceID) {
			err = membershipsunchecked.LeaveDoomedWorkspaceIn(ctx, ts, pub, userID)
		} else {
			err = memberships.LeaveIn(ctx, ts, pub, userID)
		}
		if err != nil {
			return err
		}
	}
	return usersunchecked.DeleteIn(ctx, tx, pub, u)
}
