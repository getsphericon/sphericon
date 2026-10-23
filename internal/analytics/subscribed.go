package analytics

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/reportday"
)

// SubscribedPeople is the Subscribed people report: the flow of subscriptions per Report day and
// the number of subscribed people now. There is no reconstructed running total, so the series
// never disagrees with Current after Contacts are deleted.
type SubscribedPeople struct {
	// Current is the Contacts that are not globally opted out, read from current state.
	Current int
	Series  []SubscribedPoint
}

// SubscribedPoint is one Report day: people who subscribed (marketing.confirmed Events), people
// who unsubscribed (email.unsubscribed Events) and their difference.
type SubscribedPoint struct {
	Date                          string
	Subscribed, Unsubscribed, Net int
}

// SubscribedPeople computes the report for the Workspace behind s over the window.
func (m *Module) SubscribedPeople(ctx context.Context, s *ent.Scoped, w Window) (*SubscribedPeople, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	total, err := s.Contact().Query().Count(ctx)
	if err != nil {
		return nil, err
	}
	optedOut, err := s.Contact().Query().Where(eligibility.GloballyOptedOut(eligibility.ChannelEmail)).Count(ctx)
	if err != nil {
		return nil, err
	}
	series, err := subscriptionSeries(ctx, s, days)
	if err != nil {
		return nil, err
	}
	return &SubscribedPeople{Current: total - optedOut, Series: series}, nil
}

// subscriptionSeries buckets the subscription Events by the Report day they happened on
// (occurred_at, the creation time when there is none: ADR 0044), zero-filled over the window.
// No machine filter applies: only opens and clicks are made by software (ADR 0053).
func subscriptionSeries(ctx context.Context, s *ent.Scoped, days reportday.Window) ([]SubscribedPoint, error) {
	var rows []struct {
		Day          string `sql:"day"`
		Subscribed   int    `sql:"subscribed"`
		Unsubscribed int    `sql:"unsubscribed"`
	}
	err := s.Event().Query().
		Modify(func(sel *sql.Selector) {
			moment := "COALESCE(" + sel.C(event.FieldOccurredAt) + ", " + sel.C(event.FieldCreatedAt) + ")"
			action := sel.C(event.FieldAction)
			sel.Select(
				sql.As("COUNT(*) FILTER (WHERE "+action+" = '"+events.NameMarketingConfirmed+"')", "subscribed"),
				sql.As("COUNT(*) FILTER (WHERE "+action+" = '"+events.NameEmailUnsubscribed+"')", "unsubscribed"),
			)
			sel.AppendSelectExprAs(localDay(moment, days.Loc), "day").
				Where(sql.And(
					sql.In(action, events.NameMarketingConfirmed, events.NameEmailUnsubscribed),
					// raw sql: the moment is COALESCE(occurred_at, created_at), which ent has no predicate for
					sql.P(func(b *sql.Builder) { b.WriteString(moment + " >= ").Arg(days.Since) }),
					// raw sql: the same moment, the exclusive end of the window
					sql.P(func(b *sql.Builder) { b.WriteString(moment + " < ").Arg(days.Until) }),
				)).
				GroupBy("day")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]SubscribedPoint, len(rows))
	for _, r := range rows {
		byDay[r.Day] = SubscribedPoint{Date: r.Day, Subscribed: r.Subscribed, Unsubscribed: r.Unsubscribed, Net: r.Subscribed - r.Unsubscribed}
	}
	points := make([]SubscribedPoint, 0, len(days.Dates))
	for _, day := range days.Dates {
		p := byDay[day]
		p.Date = day
		points = append(points, p)
	}
	return points, nil
}
