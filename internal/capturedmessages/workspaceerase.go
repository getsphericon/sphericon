package capturedmessages

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
)

// EraseCapturedMessagesIn removes the scoped Workspace's Captured messages with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseCapturedMessagesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.CapturedMessage().Delete().Where(capturedmessage.IDIn(ids...)).Exec(ctx)
	return err
}
