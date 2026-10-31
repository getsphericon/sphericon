package automations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// WithRevenue is an Automation and what it earned: the Orders credited to it and their net
// revenue in the Reporting currency (ADR 0064), read through the orders module.
type WithRevenue struct {
	*ent.Automation

	Revenue orders.Revenue
}

// ListWithRevenue is List with the revenue of each Automation of the page, read in one grouped
// query whatever the page's size.
func (m *Module) ListWithRevenue(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[WithRevenue], error) {
	page, err := m.List(ctx, s, p, archived)
	if err != nil {
		return pagination.Page[WithRevenue]{}, err
	}
	figures, err := orders.CreditedToAutomations(ctx, s, lo.Map(page.Items, func(a *ent.Automation, _ int) int64 { return a.ID })...)
	if err != nil {
		return pagination.Page[WithRevenue]{}, err
	}
	return pagination.Map(page, func(a *ent.Automation) WithRevenue {
		return WithRevenue{Automation: a, Revenue: figures.Of(a.ID)}
	}), nil
}

// GetWithRevenue is Get with the Automation's revenue.
func (m *Module) GetWithRevenue(ctx context.Context, s *ent.Scoped, id int64) (WithRevenue, error) {
	a, err := m.Get(ctx, s, id)
	if err != nil {
		return WithRevenue{}, err
	}
	figures, err := orders.CreditedToAutomations(ctx, s, a.ID)
	if err != nil {
		return WithRevenue{}, err
	}
	return WithRevenue{Automation: a, Revenue: figures.Of(a.ID)}, nil
}
