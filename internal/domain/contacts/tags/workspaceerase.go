package tags

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
)

// EraseTagsIn removes the scoped Workspace's Tags with the ids, inside the transaction s is bound
// to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion, ADR 0043),
// which lists the ids and checks nothing else.
func EraseTagsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Tag().Delete().Where(tag.IDIn(ids...)).Exec(ctx)
	return err
}
