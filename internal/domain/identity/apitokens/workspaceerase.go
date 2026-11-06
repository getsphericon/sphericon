package apitokens

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseAPITokensIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.ApiToken().Delete().Where(apitoken.IDIn(ids...)).Exec(ctx)
	return err
}
