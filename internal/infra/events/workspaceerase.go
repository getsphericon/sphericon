package events

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseEventsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Event().Delete().Where(event.IDIn(ids...)).Exec(ctx)
	return err
}
