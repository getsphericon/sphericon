// Package analytics computes the dashboard's Overview: contacts, Engagement rates,
// automations and the daily time series (GLOSSARY "Engagement rates").
//
// Days are Report days (ADR 0057): calendar days in the Workspace time zone, cut by
// internal/reportday, so the whole history is re-bucketed the moment the zone changes.
//
// Engagement rates are a send cohort: the denominator is the Broadcast recipients
// sent in the window, and opens and clicks are counted among that same cohort, so
// opened never exceeds sent and every rate is within [0, 1]. They are not the
// Sending-domain Complaint and Bounce rates of internal/reputation.
package analytics

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/reportday"
)

// Window is the selectable trailing window in Report days, today included.
type Window int

const (
	Window7Days  Window = 7
	Window30Days Window = 30
	Window90Days Window = 90
)

// Overview is everything the dashboard shows for one Workspace and window.
type Overview struct {
	Contacts    Contacts
	Email       Email
	Automations Automations
	Series      []Point
}

// Contacts is the contact breakdown: Unsubscribed is "globally non-mailable" (ADR 0001), Active the remainder.
type Contacts struct {
	Total, Active, Unsubscribed, NewInWindow int
}

// Email is the send cohort's counts and Engagement rates.
type Email struct {
	Sent, Opened, Clicked                int
	OpenRate, ClickRate, ClickToOpenRate float32
}

// Automations is a point-in-time snapshot.
type Automations struct {
	Total, Active, RunsActive, RunsCompleted int
}

// Point is one Report day of the send cohort.
type Point struct {
	Date                  string
	Sent, Opened, Clicked int
}

// Module computes Overviews. Its clock is time.Now.
type Module struct {
	now func() time.Time
}

func New() *Module { return &Module{now: time.Now} }

// WithClock returns the module reading now from the given clock.
func (m *Module) WithClock(now func() time.Time) *Module { return &Module{now: now} }

// days is the trailing Report-day window of w days for the Workspace behind s, read in its
// time zone; every report of the module cuts its window here.
func (m *Module) days(ctx context.Context, s *ent.Scoped, w Window) (reportday.Window, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return reportday.Window{}, err
	}
	loc, err := reportday.Location(ws.Timezone)
	if err != nil {
		return reportday.Window{}, err
	}
	return reportday.Trailing(m.now(), loc, int(w)), nil
}

// localDay is the Report-day label (YYYY-MM-DD) of a timestamp column in loc. The zone is a
// bound argument, never spliced into the statement.
func localDay(column string, loc *time.Location) sql.Querier {
	// raw sql: ent has no AT TIME ZONE expression; the zone is a bound argument
	return sql.ExprFunc(func(b *sql.Builder) {
		b.WriteString("to_char(").WriteString(column).WriteString(" AT TIME ZONE ").Arg(loc.String()).WriteString(", 'YYYY-MM-DD')")
	})
}

// Overview computes the dashboard for the Workspace behind s over the window.
func (m *Module) Overview(ctx context.Context, s *ent.Scoped, w Window) (*Overview, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	contacts, err := contactCounts(ctx, s, days.Since)
	if err != nil {
		return nil, err
	}
	email, err := emailCounts(ctx, s, days.Since, days.Until)
	if err != nil {
		return nil, err
	}
	automations, err := automationCounts(ctx, s)
	if err != nil {
		return nil, err
	}
	series, err := timeSeries(ctx, s, days)
	if err != nil {
		return nil, err
	}
	return &Overview{Contacts: contacts, Email: email, Automations: automations, Series: series}, nil
}

func contactCounts(ctx context.Context, s *ent.Scoped, since time.Time) (Contacts, error) {
	total, err := s.Contact().Query().Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	unsub, err := s.Contact().Query().Where(eligibility.GloballyOptedOut(eligibility.ChannelEmail)).Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	fresh, err := s.Contact().Query().Where(contact.CreatedAtGTE(since)).Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	return Contacts{Total: total, Active: total - unsub, Unsubscribed: unsub, NewInWindow: fresh}, nil
}

// sentBetween matches the recipients whose email was sent in [since, until): the send time is
// the Outbound message's (ADR 0015), the recipient only carries the engagement.
func sentBetween(since, until time.Time) predicate.BroadcastRecipient {
	return func(sel *sql.Selector) {
		m := sql.Table(outboundmessage.Table).As("sent")
		sel.Where(sql.Exists(
			sql.Select(m.C(outboundmessage.FieldID)).From(m).Where(sql.And(
				sql.ColumnsEQ(m.C(outboundmessage.FieldBroadcastRecipientID), sel.C(broadcastrecipient.FieldID)),
				sql.EQ(m.C(outboundmessage.FieldStatus), string(outboundmessage.StatusSent)),
				sql.GTE(m.C(outboundmessage.FieldSentAt), since),
				sql.LT(m.C(outboundmessage.FieldSentAt), until),
			)),
		))
	}
}

func emailCounts(ctx context.Context, s *ent.Scoped, since, until time.Time) (Email, error) {
	cohort := func() *ent.BroadcastRecipientQuery {
		return s.BroadcastRecipient().Query().Where(sentBetween(since, until))
	}
	sent, err := cohort().Count(ctx)
	if err != nil {
		return Email{}, err
	}
	opened, err := cohort().Where(broadcastrecipient.OpenedAtNotNil()).Count(ctx)
	if err != nil {
		return Email{}, err
	}
	clicked, err := cohort().Where(broadcastrecipient.ClickedAtNotNil()).Count(ctx)
	if err != nil {
		return Email{}, err
	}
	return Email{
		Sent: sent, Opened: opened, Clicked: clicked,
		OpenRate: ratio(opened, sent), ClickRate: ratio(clicked, sent), ClickToOpenRate: ratio(clicked, opened),
	}, nil
}

func automationCounts(ctx context.Context, s *ent.Scoped) (Automations, error) {
	total, err := s.Automation().Query().Count(ctx)
	if err != nil {
		return Automations{}, err
	}
	active, err := s.Automation().Query().Where(automation.StatusEQ(automation.StatusActive)).Count(ctx)
	if err != nil {
		return Automations{}, err
	}
	runsActive, err := s.AutomationRun().Query().Where(automationrun.StatusEQ(automationrun.StatusActive)).Count(ctx)
	if err != nil {
		return Automations{}, err
	}
	runsCompleted, err := s.AutomationRun().Query().Where(automationrun.StatusEQ(automationrun.StatusCompleted)).Count(ctx)
	if err != nil {
		return Automations{}, err
	}
	return Automations{Total: total, Active: active, RunsActive: runsActive, RunsCompleted: runsCompleted}, nil
}

// timeSeries buckets the send cohort by the Report day of its send time, then zero-fills
// every day of the window from the local dates, so a 23- or 25-hour day is one bucket.
func timeSeries(ctx context.Context, s *ent.Scoped, days reportday.Window) ([]Point, error) {
	var rows []struct {
		Day     string `sql:"day"`
		Sent    int    `sql:"sent"`
		Opened  int    `sql:"opened"`
		Clicked int    `sql:"clicked"`
	}
	err := s.BroadcastRecipient().Query().
		Modify(func(sel *sql.Selector) {
			// One message per recipient (ADR 0015): the join keeps one row per recipient.
			m := sql.Table(outboundmessage.Table).As("sent")
			sel.Join(m).On(sel.C(broadcastrecipient.FieldID), m.C(outboundmessage.FieldBroadcastRecipientID))
			sentAt := m.C(outboundmessage.FieldSentAt)
			sel.Select(
				sql.As("COUNT(*)", "sent"),
				sql.As(fmt.Sprintf("COUNT(%s)", sel.C(broadcastrecipient.FieldOpenedAt)), "opened"),
				sql.As(fmt.Sprintf("COUNT(%s)", sel.C(broadcastrecipient.FieldClickedAt)), "clicked"),
			)
			sel.AppendSelectExprAs(localDay(sentAt, days.Loc), "day").
				Where(sql.And(
					sql.EQ(m.C(outboundmessage.FieldStatus), string(outboundmessage.StatusSent)),
					sql.GTE(sentAt, days.Since), sql.LT(sentAt, days.Until),
				)).
				GroupBy("day")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]Point, len(rows))
	for _, r := range rows {
		byDay[r.Day] = Point{Date: r.Day, Sent: r.Sent, Opened: r.Opened, Clicked: r.Clicked}
	}
	points := make([]Point, 0, len(days.Dates))
	for _, day := range days.Dates {
		p := byDay[day]
		p.Date = day
		points = append(points, p)
	}
	return points, nil
}

// ratio is num/denom in [0, 1], 0 for an empty denominator.
func ratio(num, denom int) float32 {
	if denom <= 0 {
		return 0
	}
	return float32(num) / float32(denom)
}
