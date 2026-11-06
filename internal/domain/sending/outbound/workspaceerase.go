package outbound

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/trackedlink"
)

// EraseTrackedLinksIn removes the scoped Workspace's tracked links with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseTrackedLinksIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.TrackedLink().Delete().Where(trackedlink.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseOutboundMessagesIn removes the scoped Workspace's Outbound messages with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseOutboundMessagesIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.OutboundMessage().Delete().Where(outboundmessage.IDIn(ids...)).Exec(ctx)
	return err
}
