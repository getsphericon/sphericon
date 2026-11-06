package tags

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseTagsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Tag().Delete().Where(tag.IDIn(ids...)).Exec(ctx)
	return err
}
