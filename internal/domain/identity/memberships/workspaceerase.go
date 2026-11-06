package memberships

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseMembershipsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Membership().Delete().Where(membership.IDIn(ids...)).Exec(ctx)
	return err
}
