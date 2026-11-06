package capturedmessages

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseCapturedMessagesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.CapturedMessage().Delete().Where(capturedmessage.IDIn(ids...)).Exec(ctx)
	return err
}
