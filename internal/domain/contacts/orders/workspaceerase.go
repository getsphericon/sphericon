package orders

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
)

// EraseHeldOrderEventsIn removes the scoped Workspace's held Order events with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseHeldOrderEventsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.HeldOrderEvent().Delete().Where(heldorderevent.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseOrderMovementsIn removes the scoped Workspace's Order movements with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseOrderMovementsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.OrderMovement().Delete().Where(ordermovement.IDIn(ids...)).Exec(ctx)
	return err
}

// EraseOrdersIn removes the scoped Workspace's Orders with the ids, inside the transaction s is
// bound to: one batch of the erase of a deleted Workspace (internal/usecase/workspacedeletion, ADR
// 0043), which lists the ids and checks nothing else.
func EraseOrdersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Order().Delete().Where(order.IDIn(ids...)).Exec(ctx)
	return err
}
