package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// The Owners of a Workspace (GLOSSARY "Role") are defined here and nowhere else: every
// lookup, lock and notice that needs them goes through these functions.

// Owners loads the owner Memberships of the Workspace s with their Users, oldest first.
func (ms *Module) Owners(ctx context.Context, s *ent.Scoped) ([]*ent.Membership, error) {
	return s.Membership().Query().
		Where(OwnerRole()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// OwnersOf loads the owner Memberships, with their Users, of every Workspace in workspaceIDs,
// oldest first, over the module's raw client. It is the paged form of Owners for callers that hold
// no scope (a list over many Workspaces).
func (ms *Module) OwnersOf(ctx context.Context, workspaceIDs ...int64) ([]*ent.Membership, error) {
	return ms.client.Membership.Query().
		Where(membership.WorkspaceIDIn(workspaceIDs...), OwnerRole()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// lockOwners locks the owner Memberships of the Workspace s for the transaction and returns
// their ids. Locking first serializes concurrent Owner changes: the second waits, then reads
// the committed outcome of the first.
func lockOwners(ctx context.Context, s *ent.Scoped) ([]int64, error) {
	return s.Membership().Query().Where(OwnerRole()).ForUpdate().IDs(ctx)
}

// SoleOwnership is what SoleOwnershipIn read under the Owner locks: the User's Memberships in
// ascending Workspace id order (Held) and the ids of the Workspaces whose only Owner is the User
// (SoleOwned).
type SoleOwnership struct {
	Held      []*ent.Membership
	SoleOwned []int64
}

// SoleOwnershipIn returns, on the transaction tx, the User's Memberships in ascending Workspace
// id order, read after the Owner rows of each of their Workspaces are locked, and the ids of the
// Workspaces whose only Owner is the User as read under those locks (User deletion, ADR 0050).
// The Workspaces are known from a first read; the Memberships that count are read again once the
// locks are held, so a Role change or a removal that committed meanwhile is seen. A Workspace the
// second read adds (the User joined it meanwhile) is locked too, out of order, which can at worst
// deadlock and abort with a database error, never read stale. Locking in ascending Workspace id
// order makes two co-Owners leaving at once serialize on the Workspace: the later one reads the
// earlier one's outcome.
func (ms *Module) SoleOwnershipIn(ctx context.Context, tx *ent.Client, userID int64) (SoleOwnership, error) {
	read := func() ([]*ent.Membership, error) {
		return tx.Membership.Query().
			Where(membership.UserID(userID)).
			Order(ent.Asc(membership.FieldWorkspaceID)).
			All(ctx)
	}
	owners := map[int64][]int64{}
	lock := func(held []*ent.Membership) error {
		for _, m := range held {
			if _, locked := owners[m.WorkspaceID]; locked {
				continue
			}
			ids, err := lockOwners(ctx, tx.Scoped(m.WorkspaceID))
			if err != nil {
				return err
			}
			owners[m.WorkspaceID] = ids
		}
		return nil
	}

	first, err := read()
	if err != nil {
		return SoleOwnership{}, err
	}
	if err := lock(first); err != nil {
		return SoleOwnership{}, err
	}
	held, err := read()
	if err != nil {
		return SoleOwnership{}, err
	}
	if err := lock(held); err != nil {
		return SoleOwnership{}, err
	}
	sole := SoleOwnership{Held: held}
	for _, m := range held {
		if IsOwner(m.Role) && len(owners[m.WorkspaceID]) == 1 {
			sole.SoleOwned = append(sole.SoleOwned, m.WorkspaceID)
		}
	}
	return sole, nil
}
