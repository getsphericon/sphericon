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
	"github.com/getsphericon/sphericon/internal/events"
)

// SoleOwnerError is the refusal of User deletion while the User is the only Owner of a live
// Workspace (ADR 0050): nothing is deleted, and Workspaces says which ones to hand over or
// delete first. It is an ErrLastOwner, the same rule as any Owner change.
type SoleOwnerError struct {
	// Workspaces are the live Workspaces the User solely owns, in ascending id order.
	Workspaces []*ent.Workspace
}

func (e *SoleOwnerError) Error() string {
	names := lo.Map(e.Workspaces, func(w *ent.Workspace, _ int) string { return w.Name })
	return fmt.Sprintf("%s: the user is the only owner of %s", ErrLastOwner.Error(), strings.Join(names, ", "))
}

// Unwrap makes errors.Is(err, ErrLastOwner) true for the refusal.
func (e *SoleOwnerError) Unwrap() error { return ErrLastOwner }

// DeleteUser erases the User userID at once and for good (User deletion, ADR 0050), in one
// transaction, and returns the deleted User (its name and address, for the mail that tells
// the owner of the mailbox). The caller has proven who is asking; confirm runs inside the
// transaction, after the refusal check and before anything is removed, with the User's row
// locked and freshly read, so a second-factor code is spent only if the deletion commits
// (nil confirms nothing) and the caller can check that what its proof assumed still holds.
//
// The transaction locks the User row, then the Owner rows of every Workspace the User
// belongs to, in ascending Workspace id order, so two co-Owners deleting at once serialize
// on the Workspace and cannot deadlock: the later one reads the earlier one's outcome. Only
// then does it read the User's Memberships and Roles that the decision rests on, so a
// concurrent change cannot leave them stale. It refuses with a *SoleOwnerError (an
// ErrLastOwner) while the User is the only Owner of a Workspace that is not in its deletion
// window (ADR 0043); a sole-owned Workspace in the window does not block and is erased on
// schedule. Every Membership then goes through its Workspace's scope with the User as actor
// of a `membership.delete` Audit entry (Audit entries that name the User are never
// rewritten), and the User row is deleted: Linked identities and Recovery codes follow by
// cascade, an Invitation the User sent keeps its place with no sender. Sessions need
// nothing: the security handler finds no User.
func (a *Accounts) DeleteUser(ctx context.Context, userID int64, confirm func(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) error) (*ent.User, error) {
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
			return &SoleOwnerError{Workspaces: live}
		}
		if confirm != nil {
			if err := confirm(ctx, tx, pub, u); err != nil {
				return err
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
		if IsOwner(m.Role) && len(owners[m.WorkspaceID]) == 1 {
			sole = append(sole, m.WorkspaceID)
		}
	}
	return sole
}
