package orders

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/reportday"
)

// Every read of revenue is here (ADR 0064): it is summed from the Order Movements, each on the
// day of its own time, never stored on the Order, the Broadcast or the Automation. Only the
// Orders in the Reporting currency count; until one is chosen every figure is zero.

// Credited is what a Broadcast or an Automation earned (ADR 0064): the Orders credited to it,
// completed, and the net sum of their Movements, both in the Reporting currency. Nothing of it
// is stored: a refund, an update or a cancellation moves Amount on the day of its Movement.
type Credited struct {
	Orders int
	// Amount is in the minor unit of the Reporting currency.
	Amount int64
}

// Figures is the Credited of a set of Broadcasts or Automations by id, with the Reporting
// currency they are in. Currency is nil until an Owner or Admin chooses it; every figure is
// then zero.
type Figures struct {
	Currency *string
	ByID     map[int64]Credited
}

// Revenue is what one Broadcast or Automation earned, with the Reporting currency it is in: nil
// until an Owner or Admin chooses it, every figure zero until then.
type Revenue struct {
	Credited

	Currency *string
}

// Of is the Revenue of one id of the Figures: zero when nothing is credited to it.
func (f Figures) Of(id int64) Revenue {
	return Revenue{Credited: f.ByID[id], Currency: f.Currency}
}

// CreditedToBroadcasts reads the figures of the given Broadcasts in one grouped query.
func CreditedToBroadcasts(ctx context.Context, s *ent.Scoped, ids ...int64) (Figures, error) {
	return credited(ctx, s, ids, order.FieldBroadcastID, order.BroadcastIDIn(ids...))
}

// CreditedToAutomations reads the figures of the given Automations in one grouped query.
func CreditedToAutomations(ctx context.Context, s *ent.Scoped, ids ...int64) (Figures, error) {
	return credited(ctx, s, ids, order.FieldAutomationID, order.AutomationIDIn(ids...))
}

// CreditedToAutomationsBetween reads the figures of the given Automations from the Movements that
// fell in [since, until) only: the Orders completed then, and the net change of their revenue
// then. Every Movement keeps its own day (ADR 0064), so the figures of a past period do not change.
func CreditedToAutomationsBetween(ctx context.Context, s *ent.Scoped, since, until time.Time, ids ...int64) (Figures, error) {
	return credited(ctx, s, ids, order.FieldAutomationID, order.AutomationIDIn(ids...), between(since, until)...)
}

func credited(ctx context.Context, s *ent.Scoped, ids []int64, column string, ofThem predicate.Order, within ...predicate.OrderMovement) (Figures, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Figures{}, err
	}
	out := Figures{Currency: ws.ReportingCurrency, ByID: make(map[int64]Credited, len(ids))}
	for _, id := range ids {
		out.ByID[id] = Credited{}
	}
	if out.Currency == nil || len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID     int64 `sql:"credited_id"`
		Orders int   `sql:"orders"`
		Amount int64 `sql:"amount"`
	}
	err = s.OrderMovement().Query().
		Where(ordermovement.HasOrderWith(ofThem, order.Currency(*out.Currency))).
		Where(within...).
		Modify(func(sel *sql.Selector) {
			o := sql.Table(order.Table)
			sel.Join(o).On(sel.C(ordermovement.FieldOrderID), o.C(order.FieldID))
			netAndCompleted(sel)
			// raw sql: the credited Broadcast or Automation is a column of the joined Order, which ent's aggregate cannot group by from the Movement.
			sel.AppendSelectExprAs(sql.Expr(o.C(column)), "credited_id").GroupBy(o.C(column))
		}).Scan(ctx, &rows)
	if err != nil {
		return Figures{}, err
	}
	for _, r := range rows {
		out.ByID[r.ID] = Credited{Orders: r.Orders, Amount: r.Amount}
	}
	return out, nil
}

// WorkspaceRevenue is the revenue of the Orders a Workspace sent to Sphericon over a window of
// Report days, in its Reporting currency: the sum of Order Movements, each on the Report day of its
// own time.
type WorkspaceRevenue struct {
	// Currency is the Reporting currency, nil until chosen; every figure is then zero.
	Currency *string
	// Total is the sum of the Movements, in the minor unit of Currency.
	Total int64
	// Credited is the part of Total in the Movements of the Orders credited to a Broadcast or an
	// Automation.
	Credited int64
	// Orders is the Orders completed in the window, in Currency.
	Orders int
	// CreditedOrders is the Orders completed in the window that are credited to one of them.
	CreditedOrders int
	// OtherCurrencyOrders is the Orders completed in the window in another currency: stored
	// and listed, counted in no sum.
	OtherCurrencyOrders int
	Series              []RevenueDay
}

// RevenueDay is one Report day of WorkspaceRevenue, zero-filled across the window.
type RevenueDay struct {
	Date   string
	Amount int64
	Orders int
}

// WorkspaceRevenueOver reads the revenue of the Workspace behind s over the window w. Until the
// Reporting currency is chosen it answers with the currency unset and zeros, and counts every
// Order already received once it is.
func WorkspaceRevenueOver(ctx context.Context, s *ent.Scoped, w reportday.Window) (*WorkspaceRevenue, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	report := &WorkspaceRevenue{Currency: ws.ReportingCurrency}
	byDay := map[string]RevenueDay{}
	if cur := ws.ReportingCurrency; cur != nil {
		inWindow := between(w.Since, w.Until)
		var days []struct {
			Day    string `sql:"day"`
			Amount int64  `sql:"amount"`
			Orders int    `sql:"orders"`
		}
		err := s.OrderMovement().Query().
			Where(inWindow...).
			Where(ordermovement.HasOrderWith(order.Currency(*cur))).
			Modify(func(sel *sql.Selector) {
				netAndCompleted(sel)
				sel.AppendSelectExprAs(reportday.Label(sel.C(ordermovement.FieldOccurredAt), w.Loc), "day").GroupBy("day")
			}).Scan(ctx, &days)
		if err != nil {
			return nil, err
		}
		for _, r := range days {
			byDay[r.Day] = RevenueDay{Amount: r.Amount, Orders: r.Orders}
			report.Total += r.Amount
			report.Orders += r.Orders
		}
		var credited []struct {
			Amount *int64 `sql:"amount"`
			Orders int    `sql:"orders"`
		}
		err = s.OrderMovement().Query().
			Where(inWindow...).
			Where(ordermovement.HasOrderWith(order.Currency(*cur), creditedToMessages())).
			Modify(netAndCompleted).Scan(ctx, &credited)
		if err != nil {
			return nil, err
		}
		for _, r := range credited {
			report.Credited += lo.FromPtr(r.Amount)
			report.CreditedOrders += r.Orders
		}
		report.OtherCurrencyOrders, err = s.OrderMovement().Query().
			Where(inWindow...).
			Where(ordermovement.KindEQ(ordermovement.KindCompleted), ordermovement.HasOrderWith(order.CurrencyNEQ(*cur))).
			Count(ctx)
		if err != nil {
			return nil, err
		}
	}
	report.Series = make([]RevenueDay, 0, len(w.Dates))
	for _, day := range w.Dates {
		p := byDay[day]
		p.Date = day
		report.Series = append(report.Series, p)
	}
	return report, nil
}

// creditedToMessages is an Order credited to a Broadcast or an Automation: the one definition of
// credited (see credit).
func creditedToMessages() predicate.Order {
	return order.Or(order.BroadcastIDNotNil(), order.AutomationIDNotNil())
}

// between is the Movements that fell in [since, until).
func between(since, until time.Time) []predicate.OrderMovement {
	return []predicate.OrderMovement{ordermovement.OccurredAtGTE(since), ordermovement.OccurredAtLT(until)}
}

// netAndCompleted selects, of the Movements of sel, their net amount as `amount` and the Orders
// completed among them as `orders`: a Movement of kind completed is one Order.
func netAndCompleted(sel *sql.Selector) {
	sel.Select(
		sql.As(sql.Sum(sel.C(ordermovement.FieldAmount)), "amount"),
		// raw sql: a count of the completed Movements only, which ent has no filtered aggregate for.
		sql.As("COUNT(*) FILTER (WHERE "+sel.C(ordermovement.FieldKind)+" = '"+string(ordermovement.KindCompleted)+"')", "orders"),
	)
}
