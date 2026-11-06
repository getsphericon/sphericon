package segments

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/segment"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseSegmentsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Segment().Purge().Where(segment.IDIn(ids...)).Exec(ctx)
	return err
}
