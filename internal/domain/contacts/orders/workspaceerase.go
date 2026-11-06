package orders

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/heldorderevent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseHeldOrderEventsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.HeldOrderEvent().Delete().Where(heldorderevent.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseOrderMovementsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.OrderMovement().Delete().Where(ordermovement.IDIn(ids...)).Exec(ctx)
	return err
}

func EraseOrdersIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Order().Delete().Where(order.IDIn(ids...)).Exec(ctx)
	return err
}
