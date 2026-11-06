package joindomains

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/joindomain"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseJoinDomainsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.JoinDomain().Delete().Where(joindomain.IDIn(ids...)).Exec(ctx)
	return err
}
