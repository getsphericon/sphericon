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
// Orders in the Reporting currency count; until the first Order sets it, or an Owner or Admin
// chooses it, every figure is zero.

// Credited is what a Broadcast or an Automation earned (ADR 0064): the Orders credited to it,
// completed, and the net sum of their Movements, both in the Reporting currency. Nothing of it
// is stored: a refund, an update or a cancellation moves Amount on the day of its Movement.
type Credited struct {
	Orders int
	// Amount is in the minor unit of the Reporting currency.
	Amount int64
}

// Figures is the Credited of a set of Broadcasts or Automations by id, with the Reporting
// currency they are in. Currency is nil until the first Order sets it or an Owner or Admin
// chooses it; every figure is then zero.
type Figures struct {
	Currency *string
	ByID     map[int64]Credited
}

// Revenue is what one Broadcast or Automation earned, with the Reporting currency it is in: nil,
// and every figure zero, until the first Order sets it or an Owner or Admin chooses it.
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
// Report days and over the period just before it, in its Reporting currency: the sum of Order
// Movements, each on the Report day of its own time.
type WorkspaceRevenue struct {
	// Currency is the Reporting currency, nil until the first Order sets it or an Owner or Admin
	// chooses it; every figure is then zero.
	Currency *string
	// FirstOrderAt is when the Workspace's first Order was completed, in any currency and outside
	// the window too: nil while it has sent none, whatever Currency is.
	FirstOrderAt *time.Time
	// Current is the window; Previous is as many Report days just before it
	// (reportday.Window.Previous, the rule of the Automation report), both in Currency.
	Current, Previous RevenuePeriod
	// Series is the current period by Report day.
	Series []RevenueDay
}

// RevenuePeriod is the revenue of one period of Report days.
type RevenuePeriod struct {
	// From and To are its first and last Report day, YYYY-MM-DD.
	From, To string
	// Total is the sum of the Movements, in the minor unit of the Reporting currency.
	Total int64
	// Credited is the part of Total in the Movements of the Orders credited to a Broadcast or an
	// Automation.
	Credited int64
	// Orders is the Orders completed in the period, in the Reporting currency.
	Orders int
	// CreditedOrders is the Orders completed in the period that are credited to one of them.
	CreditedOrders int
	// OtherCurrencyOrders is the Orders completed in the period in another currency, one entry
	// per currency ordered by its code: stored and listed, counted in no sum.
	OtherCurrencyOrders []CurrencyOrders
}

// CurrencyOrders is the Orders completed in one currency.
type CurrencyOrders struct {
	Currency string
	Orders   int
}

// RevenueDay is one Report day of WorkspaceRevenue, zero-filled across the window.
type RevenueDay struct {
	Date   string
	Amount int64
	Orders int
}

// WorkspaceRevenueOver reads the revenue of the Workspace behind s over the window w and the period
// just before it. Until the Reporting currency is set it answers with the currency unset and zeros.
func WorkspaceRevenueOver(ctx context.Context, s *ent.Scoped, w reportday.Window) (*WorkspaceRevenue, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	firstOrderAt, err := firstOrderAt(ctx, s)
	if err != nil {
		return nil, err
	}
	current, byDay, err := revenuePeriodOf(ctx, s, ws.ReportingCurrency, w)
	if err != nil {
		return nil, err
	}
	prev := w.Previous()
	previous, _, err := revenuePeriodOf(ctx, s, ws.ReportingCurrency, prev)
	if err != nil {
		return nil, err
	}
	report := &WorkspaceRevenue{Currency: ws.ReportingCurrency, FirstOrderAt: firstOrderAt, Current: current, Previous: previous}
	report.Series = make([]RevenueDay, 0, len(w.Dates))
	for _, day := range w.Dates {
		p := byDay[day]
		p.Date = day
		report.Series = append(report.Series, p)
	}
	return report, nil
}

// revenuePeriodOf sums the Movements of the period w in the Reporting currency cur, and returns
// them by Report day too. With no currency every figure is zero.
func revenuePeriodOf(ctx context.Context, s *ent.Scoped, cur *string, w reportday.Window) (RevenuePeriod, map[string]RevenueDay, error) {
	out := RevenuePeriod{From: w.First(), To: w.Last()}
	byDay := map[string]RevenueDay{}
	if cur == nil {
		return out, byDay, nil
	}
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
		return RevenuePeriod{}, nil, err
	}
	for _, r := range days {
		byDay[r.Day] = RevenueDay{Amount: r.Amount, Orders: r.Orders}
		out.Total += r.Amount
		out.Orders += r.Orders
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
		return RevenuePeriod{}, nil, err
	}
	for _, r := range credited {
		out.Credited += lo.FromPtr(r.Amount)
		out.CreditedOrders += r.Orders
	}
	out.OtherCurrencyOrders, err = otherCurrencyOrders(ctx, s, *cur, inWindow)
	if err != nil {
		return RevenuePeriod{}, nil, err
	}
	return out, byDay, nil
}

// firstOrderAt is the time of the Workspace's first completed Order, in any currency: nil while
// there is none.
func firstOrderAt(ctx context.Context, s *ent.Scoped) (*time.Time, error) {
	first, err := s.OrderMovement().Query().
		Where(ordermovement.KindEQ(ordermovement.KindCompleted)).
		Order(ent.Asc(ordermovement.FieldOccurredAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &first.OccurredAt, nil
}

// otherCurrencyOrders counts the Orders completed in the window in a currency other than cur,
// per currency, ordered by its code.
func otherCurrencyOrders(ctx context.Context, s *ent.Scoped, cur string, inWindow []predicate.OrderMovement) ([]CurrencyOrders, error) {
	var rows []struct {
		Currency string `sql:"currency"`
		Orders   int    `sql:"orders"`
	}
	err := s.OrderMovement().Query().
		Where(inWindow...).
		Where(ordermovement.KindEQ(ordermovement.KindCompleted), ordermovement.HasOrderWith(order.CurrencyNEQ(cur))).
		Modify(func(sel *sql.Selector) {
			o := sql.Table(order.Table)
			sel.Join(o).On(sel.C(ordermovement.FieldOrderID), o.C(order.FieldID))
			sel.Select(sql.As(o.C(order.FieldCurrency), "currency"), sql.As(sql.Count("*"), "orders")).
				GroupBy(o.C(order.FieldCurrency)).
				OrderBy(o.C(order.FieldCurrency))
		}).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]CurrencyOrders, 0, len(rows))
	for _, r := range rows {
		out = append(out, CurrencyOrders(r))
	}
	return out, nil
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
