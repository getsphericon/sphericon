package broadcasts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
)

// EraseBroadcastRecipientsIn removes the scoped Workspace's Broadcast recipients with the ids,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseBroadcastRecipientsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseBroadcastsIn removes the scoped Workspace's Broadcasts with the ids, archived or not, inside
// the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseBroadcastsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Broadcast().Purge().Where(broadcast.IDIn(ids...)).Exec(ctx)
	return err
}
