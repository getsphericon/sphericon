package analytics

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/orders"
)

// Revenue reads the revenue of the Orders the Workspace behind s sent to Sphericon (ADR 0064)
// over the window of Report days ending today in its time zone. The orders module reads it.
func (m *Module) Revenue(ctx context.Context, s *ent.Scoped, w Window) (*orders.WorkspaceRevenue, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	return orders.WorkspaceRevenueOver(ctx, s, days)
}
