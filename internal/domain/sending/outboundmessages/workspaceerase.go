package outboundmessages

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/trackedlink"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseTrackedLinksIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.TrackedLink().Delete().Where(trackedlink.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseOutboundMessagesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.OutboundMessage().Delete().Where(outboundmessage.IDIn(ids...)).Exec(ctx)
	return err
}
