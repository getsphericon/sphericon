package accounts

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/accounts/roles"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The rules that refuse a User deletion (ADR 0050). A deletion answers each as a
// *DeletionRefusedError, whose answer names the Workspaces the User solely owns.
var (
	// ErrDeletionEmailMismatch: the email typed to confirm is not the User's own.
	ErrDeletionEmailMismatch = validation.RefuseField("email", "accounts: the typed email is not the user's", i18n.ErrorsDeleteUserEmailMismatch)
	// ErrDeletionUsePassword: a deletion link asked for a User who has a password, which
	// confirms the deletion instead.
	ErrDeletionUsePassword = validation.Refuse("accounts: the user confirms the deletion with the password", i18n.ErrorsDeleteUserUsePassword)
	// ErrProofStale: the proof no longer fits the User: the User's password was set or removed
	// between the proof and the transaction that locked the row.
	ErrProofStale = validation.Refuse("accounts: the deletion proof no longer fits the user", i18n.ErrorsDeleteUserProofStale)
	// ErrSoleOwner: the User is the only Owner of a live Workspace, the rule of any Owner change.
	ErrSoleOwner = validation.Refuse("accounts: the user is the only owner of live workspaces", i18n.ErrorsDeleteUserOnlyOwner)
)

// DeletionRefusedError is a User deletion refused by Reason, one of the rules above: nothing is
// deleted. For ErrSoleOwner, Workspaces says which Workspaces to hand over or delete first.
// errors.Is matches Reason.
type DeletionRefusedError struct {
	Reason *validation.RefusalError
	// Workspaces are the live Workspaces the User solely owns, in ascending id order.
	Workspaces []*ent.Workspace
}

func (e *DeletionRefusedError) Error() string {
	if len(e.Workspaces) == 0 {
		return e.Reason.Error()
	}
	names := lo.Map(e.Workspaces, func(w *ent.Workspace, _ int) string { return w.Name })
	return fmt.Sprintf("%s: %s", e.Reason.Error(), strings.Join(names, ", "))
}

// Unwrap is the rule that refused the deletion.
func (e *DeletionRefusedError) Unwrap() error { return e.Reason }

// refuseDeletion is the deletion refused by reason.
func refuseDeletion(reason *validation.RefusalError) error {
	return &DeletionRefusedError{Reason: reason}
}

// DeletionSubject is the User userID who asks to delete themself, once typedEmail, the email
// they typed to confirm, is theirs (ErrDeletionEmailMismatch otherwise).
func (a *Accounts) DeletionSubject(ctx context.Context, userID int64, typedEmail string) (*ent.User, error) {
	u, err := a.User(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(typedEmail), u.Email) {
		return nil, refuseDeletion(ErrDeletionEmailMismatch)
	}
	return u, nil
}

// DeleteConfirm is the spending of a second-factor code inside DeleteUser's transaction, with
// the User's row locked and freshly read. The transaction's client stays inside the closure's
// builder (the second-factor module), never in a handler.
type DeleteConfirm func(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) error

// DeleteProof is what the caller's proof of a deletion assumed, re-checked inside the
// transaction: HadPassword is whether the User had a password when the proof was read, and
// Confirm (nil for none) spends the second-factor code.
type DeleteProof struct {
	HadPassword bool
	Confirm     DeleteConfirm
}

// DeleteUser erases the User userID at once and for good (User deletion, ADR 0050), in one
// transaction, and returns the deleted User (its name and address, for the mail that tells
// the owner of the mailbox). The caller has proven who is asking; proof is re-checked inside
// the transaction, after the refusal check and before anything is removed, with the User's
// row locked and freshly read: a password set or removed since the proof was read is
// ErrProofStale, and a second-factor code is spent only if the deletion commits (a nil proof
// checks nothing).
//
// The transaction locks the User row, then the Owner rows of every Workspace the User
// belongs to, in ascending Workspace id order, so two co-Owners deleting at once serialize
// on the Workspace and cannot deadlock: the later one reads the earlier one's outcome. Only
// then does it read the User's Memberships and Roles that the decision rests on, so a
// concurrent change cannot leave them stale. It refuses with ErrSoleOwner (naming the
// Workspaces) while the User is the only Owner of a Workspace that is not in its deletion
// window (ADR 0043); a sole-owned Workspace in the window does not block and is erased on
// schedule. Every Membership then goes through its Workspace's scope with the User as actor
// of a `membership.delete` Audit entry (Audit entries that name the User are never
// rewritten), and the User row is deleted: Linked identities and Recovery codes follow by
// cascade, an Invitation the User sent keeps its place with no sender. Sessions need
// nothing: the security handler finds no User.
func (a *Accounts) DeleteUser(ctx context.Context, userID int64, proof *DeleteProof) (*ent.User, error) {
	var deleted *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		memberships, owners, err := membershipsUnderOwnerLocks(ctx, tx, userID)
		if err != nil {
			return err
		}
		soleOwned := soleOwnedWorkspaces(memberships, owners)
		live, err := tx.Workspace.Query().Where(workspace.IDIn(soleOwned...), Live()).Order(ent.Asc(workspace.FieldID)).All(ctx)
		if err != nil {
			return err
		}
		if len(live) > 0 {
			return &DeletionRefusedError{Reason: ErrSoleOwner, Workspaces: live}
		}
		if proof != nil {
			if (u.PasswordHash != "") != proof.HadPassword {
				return refuseDeletion(ErrProofStale)
			}
			if proof.Confirm != nil {
				if err := proof.Confirm(ctx, tx, pub, u); err != nil {
					return err
				}
			}
		}
		for _, m := range memberships {
			ts := tx.Scoped(m.WorkspaceID)
			// Only a sole-owned Workspace still standing here is in its deletion window.
			if slices.Contains(soleOwned, m.WorkspaceID) {
				err = leaveDoomedWorkspaceIn(ctx, ts, pub, userID)
			} else {
				err = LeaveIn(ctx, ts, pub, userID)
			}
			if err != nil {
				return err
			}
		}
		if err := tx.User.DeleteOneID(userID).Exec(ctx); err != nil {
			return err
		}
		deleted = u
		return nil
	})
	return deleted, err
}

// membershipsUnderOwnerLocks returns the User's Memberships in ascending Workspace id order,
// read after the Owner rows of each of their Workspaces are locked, with the Owner Membership
// ids of each Workspace as read under that lock. The Workspaces are known from a first read;
// the Memberships that count are read again once the locks are held, so a Role change or a
// removal that committed meanwhile is seen. A Workspace the second read adds (the User
// joined it meanwhile) is locked too, out of order, which can at worst deadlock and abort
// with a database error, never read stale.
func membershipsUnderOwnerLocks(ctx context.Context, tx *ent.Client, userID int64) ([]*ent.Membership, map[int64][]int64, error) {
	read := func() ([]*ent.Membership, error) {
		return tx.Membership.Query().
			Where(membership.UserID(userID)).
			Order(ent.Asc(membership.FieldWorkspaceID)).
			All(ctx)
	}
	owners := map[int64][]int64{}
	lock := func(memberships []*ent.Membership) error {
		for _, m := range memberships {
			if _, locked := owners[m.WorkspaceID]; locked {
				continue
			}
			ids, err := LockOwners(ctx, tx.Scoped(m.WorkspaceID))
			if err != nil {
				return err
			}
			owners[m.WorkspaceID] = ids
		}
		return nil
	}

	first, err := read()
	if err != nil {
		return nil, nil, err
	}
	if err := lock(first); err != nil {
		return nil, nil, err
	}
	memberships, err := read()
	if err != nil {
		return nil, nil, err
	}
	if err := lock(memberships); err != nil {
		return nil, nil, err
	}
	return memberships, owners, nil
}

// soleOwnedWorkspaces returns the ids of the Workspaces whose only Owner is the Membership's
// own.
func soleOwnedWorkspaces(memberships []*ent.Membership, owners map[int64][]int64) []int64 {
	var sole []int64
	for _, m := range memberships {
		if roles.IsOwner(m.Role) && len(owners[m.WorkspaceID]) == 1 {
			sole = append(sole, m.WorkspaceID)
		}
	}
	return sole
}
