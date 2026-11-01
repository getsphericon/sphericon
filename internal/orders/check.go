package orders

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/internal/events"
)

// Check refuses, at ingest, an order Event (ADR 0064) whose amounts the Workspace cannot count:
// an Order Completed without a currency, more places than its currency has, an unknown currency
// code or no order_id. Every other action passes. An Updated, Refunded or Cancelled that names no
// currency is checked against the currency of its Order when the Order exists, and only as a
// number otherwise.
type Check struct {
	s *ent.Scoped
}

// NewCheck is the check for the Workspace behind s.
func NewCheck(s *ent.Scoped) *Check { return &Check{s: s} }

// Check refuses an order Event with events.ErrRejected made With why.
func (c *Check) Check(ctx context.Context, action string, props map[string]any) error {
	if !IsOrderAction(action) {
		return nil
	}
	fallback, err := c.orderCurrency(ctx, action, props)
	if err != nil {
		return err
	}
	if _, err := Parse(action, props, fallback); err != nil {
		return events.Reject(err.Error())
	}
	return nil
}

// orderCurrency is the currency an Event without one is read in: its Order's, nil for an Order
// Completed, an Event that names its own or an Order not yet known.
func (c *Check) orderCurrency(ctx context.Context, action string, props map[string]any) (*string, error) {
	if action == ActionCompleted || props[propCurrency] != nil {
		return nil, nil
	}
	orderID, ok := props[propOrderID].(string)
	if !ok || orderID == "" {
		return nil, nil
	}
	found, err := c.s.Order().Query().Where(order.OrderID(orderID)).Select(order.FieldCurrency).Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read the currency of order %q: %w", orderID, err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	return &found[0], nil
}
