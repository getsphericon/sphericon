package analytics

import (
	"context"
	"slices"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/samber/lo"
)

// Revenue is the revenue of the Orders a Workspace sent to Sphericon over a window (ADR 0064),
// in its Reporting currency: the sum of Order Movements, each on the Report day of its own time.
type Revenue struct {
	// Currency is the Reporting currency, nil until chosen; every figure is then zero.
	Currency *string
	// Total is the sum of the Movements, in the minor unit of Currency.
	Total int64
	// Credited is the part of Total in the Movements of the Orders credited to a Broadcast or an
	// Automation (ADR 0064).
	Credited int64
	// Orders is the Orders completed in the window, in Currency.
	Orders int
	// CreditedOrders is the Orders completed in the window that are credited to one of them.
	CreditedOrders int
	// OtherCurrencyOrders is the Orders completed in the window in another currency: stored
	// and listed, counted in no sum.
	OtherCurrencyOrders int
	Series              []RevenuePoint
}

// RevenuePoint is one Report day of Revenue, zero-filled across the window.
type RevenuePoint struct {
	Date   string
	Amount int64
	Orders int
}

// Revenue reads the revenue of the Workspace behind s over the window. Until the Reporting
// currency is chosen it answers with the currency unset and zeros, and counts every Order
// already received once it is.
func (m *Module) Revenue(ctx context.Context, s *ent.Scoped, w Window) (*Revenue, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	report := &Revenue{Currency: ws.ReportingCurrency}
	byDay := map[string]RevenuePoint{}
	if cur := ws.ReportingCurrency; cur != nil {
		counted := make([]predicate.OrderMovement, 0, 4)
		counted = append(counted,
			ordermovement.OccurredAtGTE(days.Since), ordermovement.OccurredAtLT(days.Until),
			ordermovement.HasOrderWith(order.Currency(*cur)),
		)
		var amounts []struct {
			Day    string `sql:"day"`
			Amount int64  `sql:"amount"`
		}
		err := s.OrderMovement().Query().Where(counted...).
			Modify(func(sel *sql.Selector) {
				sel.Select(sql.As(sql.Sum(sel.C(ordermovement.FieldAmount)), "amount")).
					AppendSelectExprAs(reportday.Label(sel.C(ordermovement.FieldOccurredAt), days.Loc), "day").
					GroupBy("day")
			}).Scan(ctx, &amounts)
		if err != nil {
			return nil, err
		}
		for _, r := range amounts {
			p := byDay[r.Day]
			p.Amount = r.Amount
			byDay[r.Day] = p
			report.Total += r.Amount
		}
		credited := append(slices.Clone(counted), ordermovement.HasOrderWith(creditedToMessages()))
		var sums []struct {
			Amount *int64 `sql:"amount"`
		}
		err = s.OrderMovement().Query().Where(credited...).
			Modify(func(sel *sql.Selector) {
				sel.Select(sql.As(sql.Sum(sel.C(ordermovement.FieldAmount)), "amount"))
			}).Scan(ctx, &sums)
		if err != nil {
			return nil, err
		}
		for _, r := range sums {
			report.Credited += lo.FromPtr(r.Amount)
		}
		report.CreditedOrders, err = s.OrderMovement().Query().
			Where(append(credited, ordermovement.KindEQ(ordermovement.KindCompleted))...).Count(ctx)
		if err != nil {
			return nil, err
		}
		var completed []struct {
			Day    string `sql:"day"`
			Orders int    `sql:"orders"`
		}
		err = s.OrderMovement().Query().Where(append(counted, ordermovement.KindEQ(ordermovement.KindCompleted))...).
			Modify(func(sel *sql.Selector) {
				sel.Select(sql.As(sql.Count("*"), "orders")).
					AppendSelectExprAs(reportday.Label(sel.C(ordermovement.FieldOccurredAt), days.Loc), "day").
					GroupBy("day")
			}).Scan(ctx, &completed)
		if err != nil {
			return nil, err
		}
		for _, r := range completed {
			p := byDay[r.Day]
			p.Orders = r.Orders
			byDay[r.Day] = p
			report.Orders += r.Orders
		}
		report.OtherCurrencyOrders, err = s.OrderMovement().Query().
			Where(
				ordermovement.OccurredAtGTE(days.Since), ordermovement.OccurredAtLT(days.Until),
				ordermovement.KindEQ(ordermovement.KindCompleted),
				ordermovement.HasOrderWith(order.CurrencyNEQ(*cur)),
			).Count(ctx)
		if err != nil {
			return nil, err
		}
	}
	report.Series = make([]RevenuePoint, 0, len(days.Dates))
	for _, day := range days.Dates {
		p := byDay[day]
		p.Date = day
		report.Series = append(report.Series, p)
	}
	return report, nil
}

// creditedToMessages is an Order credited to a Broadcast or an Automation (ADR 0064).
func creditedToMessages() predicate.Order {
	return order.Or(order.BroadcastIDNotNil(), order.AutomationIDNotNil())
}
