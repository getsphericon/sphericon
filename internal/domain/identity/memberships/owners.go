package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// The Owners of a Workspace (GLOSSARY "Role") are defined here and nowhere else: every
// lookup, lock and notice that needs them goes through these functions.

// Owners loads the owner Memberships of the Workspace s with their Users, oldest first.
func Owners(ctx context.Context, s *ent.Scoped) ([]*ent.Membership, error) {
	return s.Membership().Query().
		Where(OwnerRole()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// OwnersOf loads the owner Memberships, with their Users, of every Workspace in workspaceIDs,
// oldest first. It is the paged form of Owners for callers that hold no scope (a list over
// many Workspaces).
func OwnersOf(ctx context.Context, client *ent.Client, workspaceIDs ...int64) ([]*ent.Membership, error) {
	return client.Membership.Query().
		Where(membership.WorkspaceIDIn(workspaceIDs...), OwnerRole()).
		WithUser().
		Order(ent.Asc(membership.FieldID)).
		All(ctx)
}

// LockOwners locks the owner Memberships of the Workspace s for the transaction and returns
// their ids. Locking first serializes concurrent Owner changes: the second waits, then reads
// the committed outcome of the first.
func LockOwners(ctx context.Context, s *ent.Scoped) ([]int64, error) {
	return s.Membership().Query().Where(OwnerRole()).ForUpdate().IDs(ctx)
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
func SoleOwnershipIn(ctx context.Context, tx *ent.Client, userID int64) (held []*ent.Membership, soleOwned []int64, err error) {
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
	if held, err = read(); err != nil {
		return nil, nil, err
	}
	if err := lock(held); err != nil {
		return nil, nil, err
	}
	for _, m := range held {
		if IsOwner(m.Role) && len(owners[m.WorkspaceID]) == 1 {
			soleOwned = append(soleOwned, m.WorkspaceID)
		}
	}
	return held, soleOwned, nil
}
