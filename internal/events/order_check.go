package events

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/orders"
)

// OrderCheck refuses, at ingest, an order Event (ADR 0064) whose amounts the Workspace cannot
// count: more places than its currency has, an unknown currency code or no order_id. Every
// other action passes. The Workspace's Reporting currency, which an Event without a `currency`
// is checked against, is read once, and only when an order Event is checked.
type OrderCheck struct {
	s         *ent.Scoped
	loaded    bool
	reporting *string
}

// NewOrderCheck is the check for the Workspace behind s.
func NewOrderCheck(s *ent.Scoped) *OrderCheck { return &OrderCheck{s: s} }

// Check refuses an order Event with a *RejectionError that says why.
func (c *OrderCheck) Check(ctx context.Context, action string, props map[string]any) error {
	if !orders.IsOrderAction(action) {
		return nil
	}
	if !c.loaded {
		w, err := c.s.Workspace(ctx)
		if err != nil {
			return fmt.Errorf("read the Reporting currency: %w", err)
		}
		c.reporting, c.loaded = w.ReportingCurrency, true
	}
	if _, err := orders.Parse(action, props, c.reporting); err != nil {
		return &RejectionError{Reason: err.Error()}
	}
	return nil
}
