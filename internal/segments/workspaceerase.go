package segments

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/segment"
)

// EraseSegmentsIn removes the scoped Workspace's Segments with the ids, archived or not, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseSegmentsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Segment().Purge().Where(segment.IDIn(ids...)).Exec(ctx)
	return err
}
