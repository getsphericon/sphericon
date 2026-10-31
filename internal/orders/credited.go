package orders

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/order"
	"github.com/getsphericon/sphericon/ent/ordermovement"
	"github.com/getsphericon/sphericon/ent/predicate"
)

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
	return credited(ctx, s, ids, order.FieldAutomationID, order.AutomationIDIn(ids...),
		ordermovement.OccurredAtGTE(since), ordermovement.OccurredAtLT(until))
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
			sel.Join(o).On(sel.C(ordermovement.FieldOrderID), o.C(order.FieldID)).
				Select(sql.As(sql.Sum(sel.C(ordermovement.FieldAmount)), "amount"),
					// raw sql: a count of the completed Movements only, which ent has no filtered aggregate for.
					sql.As("COUNT(*) FILTER (WHERE "+sel.C(ordermovement.FieldKind)+" = '"+string(ordermovement.KindCompleted)+"')", "orders")).
				// raw sql: the credited Broadcast or Automation is a column of the joined Order, which ent's aggregate cannot group by from the Movement.
				AppendSelectExprAs(sql.Expr(o.C(column)), "credited_id").
				GroupBy(o.C(column))
		}).Scan(ctx, &rows)
	if err != nil {
		return Figures{}, err
	}
	for _, r := range rows {
		out.ByID[r.ID] = Credited{Orders: r.Orders, Amount: r.Amount}
	}
	return out, nil
}
