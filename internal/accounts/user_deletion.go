package accounts

import (
	"context"
	"fmt"
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
// locked, so a second-factor code is spent only if the deletion commits (nil confirms
// nothing).
//
// The transaction locks the User row, then the Owner rows of every Workspace the User
// belongs to, in ascending Workspace id order, so two co-Owners deleting at once serialize
// on the Workspace and cannot deadlock: the later one reads the earlier one's outcome. It
// refuses with a *SoleOwnerError (an ErrLastOwner) while the User is the only Owner of a
// Workspace that is not in its deletion window (ADR 0043); a sole-owned Workspace in the
// window does not block and is erased on schedule. Every Membership then goes through its
// Workspace's scope with the User as actor of a `membership.delete` Audit entry (Audit
// entries that name the User are never rewritten), and the User row is deleted: Linked
// identities and Recovery codes follow by cascade, an Invitation the User sent keeps its
// place with no sender. Sessions need nothing: the security handler finds no User.
func (a *Accounts) DeleteUser(ctx context.Context, userID int64, confirm func(ctx context.Context, tx *ent.Client, pub events.Publisher, u *ent.User) error) (*ent.User, error) {
	var deleted *ent.User
	err := a.bus.WithinTx(ctx, func(tx *ent.Client, pub events.Publisher) error {
		u, err := tx.User.Query().Where(user.ID(userID)).ForUpdate().Only(ctx)
		if err != nil {
			return err
		}
		memberships, err := tx.Membership.Query().
			Where(membership.UserID(userID)).
			Order(ent.Asc(membership.FieldWorkspaceID)).
			All(ctx)
		if err != nil {
			return err
		}
		soleOwned, err := lockOwners(ctx, tx, memberships)
		if err != nil {
			return err
		}
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
		sole := make(map[int64]bool, len(soleOwned))
		for _, id := range soleOwned {
			sole[id] = true
		}
		for _, m := range memberships {
			// Only a sole-owned Workspace still standing here is in its deletion window.
			if err := leaveIn(ctx, tx.Scoped(m.WorkspaceID), pub, userID, sole[m.WorkspaceID]); err != nil {
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

// lockOwners locks the Owner rows of the Workspace of every Membership, in the order given
// (ascending Workspace id), and returns the ids of the Workspaces whose only Owner is the
// Membership's own.
func lockOwners(ctx context.Context, tx *ent.Client, memberships []*ent.Membership) ([]int64, error) {
	var sole []int64
	for _, m := range memberships {
		owners, err := tx.Scoped(m.WorkspaceID).Membership().Query().
			Where(membership.RoleEQ(membership.RoleOwner)).
			ForUpdate().
			IDs(ctx)
		if err != nil {
			return nil, err
		}
		if m.Role == membership.RoleOwner && len(owners) == 1 {
			sole = append(sole, m.WorkspaceID)
		}
	}
	return sole, nil
}
