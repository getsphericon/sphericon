package analytics

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
)

// EventAction is one action of the Events report: how often it happened in the window, for how
// many distinct Contacts (an anonymous Event counts as an occurrence, not as a Contact) and when
// it last happened.
type EventAction struct {
	Action   string
	Count    int
	Contacts int
	LastAt   time.Time
}

// Events groups the Events of the window by action, most frequent first. Reserved `email.*`
// actions and customer-tracked ones are both listed; machine opens and clicks are left out
// (ADR 0053). An Event is placed at the moment it happened, `occurred_at` and when it has none
// its creation time (ADR 0044), so backfilled history lands on the day it happened.
func (m *Module) Events(ctx context.Context, s *ent.Scoped, w Window) ([]EventAction, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Action   string    `sql:"action"`
		Count    int       `sql:"count"`
		Contacts int       `sql:"contacts"`
		LastAt   time.Time `sql:"last_at"`
	}
	err = s.Event().Query().
		Modify(func(sel *sql.Selector) {
			events.HumanOnly(sel)
			// raw sql: an Event's moment is occurred_at falling back to created_at, which ent's
			// predicates and aggregates cannot express as one expression.
			moment := "COALESCE(" + sel.C(event.FieldOccurredAt) + ", " + sel.C(event.FieldCreatedAt) + ")"
			// raw sql: the window bounds compare that same coalesced expression, bound as arguments.
			sel.Where(sql.P(func(b *sql.Builder) {
				b.WriteString(moment).WriteString(" >= ").Arg(days.Since).
					WriteString(" AND ").WriteString(moment).WriteString(" < ").Arg(days.Until)
			}))
			action := sel.C(event.FieldAction)
			sel.Select(
				sql.As(action, "action"),
				sql.As(sql.Count("*"), "count"),
				sql.As(sql.Count(sql.Distinct(sel.C(event.FieldContactID))), "contacts"),
				sql.As("MAX("+moment+")", "last_at"),
			).GroupBy(action)
			// raw sql: order by the aggregate's alias, which ent's order terms cannot name here.
			sel.OrderExpr(sql.Expr("count DESC, " + action))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make([]EventAction, len(rows))
	for i, r := range rows {
		out[i] = EventAction{Action: r.Action, Count: r.Count, Contacts: r.Contacts, LastAt: r.LastAt}
	}
	return out, nil
}
