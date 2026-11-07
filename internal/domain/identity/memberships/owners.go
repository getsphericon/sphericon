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
