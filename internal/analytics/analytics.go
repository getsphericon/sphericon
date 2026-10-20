// Package analytics computes the dashboard's Overview: contacts, Engagement rates,
// automations and the daily time series (GLOSSARY "Engagement rates").
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
)

// Window is the selectable trailing window in UTC days, today included.
type Window int

const (
	Window7Days  Window = 7
	Window30Days Window = 30
	Window90Days Window = 90
)

const dayFormat = "2006-01-02"

// Overview is everything the dashboard shows for one Workspace and window.
type Overview struct {
	Contacts    Contacts
	Email       Email
	Automations Automations
	Series      []Point
}

// Contacts: Unsubscribed is "globally non-mailable" (ADR 0001), Active the remainder.
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

// Point is one UTC day of the send cohort.
type Point struct {
	Date                  string
	Sent, Opened, Clicked int
}

// Module computes Overviews. Its clock is time.Now.
type Module struct {
	now func() time.Time
}

func New() *Module { return &Module{now: time.Now} }

// Overview computes the dashboard for the Workspace behind s over the window.
func (m *Module) Overview(ctx context.Context, s *ent.Scoped, w Window) (*Overview, error) {
	today := m.now().UTC().Truncate(24 * time.Hour)
	since := today.AddDate(0, 0, -(int(w) - 1))
	until := today.AddDate(0, 0, 1) // exclusive: midnight tomorrow UTC

	contacts, err := contactCounts(ctx, s, since)
	if err != nil {
		return nil, err
	}
	email, err := emailCounts(ctx, s, since, until)
	if err != nil {
		return nil, err
	}
	automations, err := automationCounts(ctx, s)
	if err != nil {
		return nil, err
	}
	series, err := timeSeries(ctx, s, since, until)
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
	total, err := s.Automation().QueryWithDiscarded().Count(ctx)
	if err != nil {
		return Automations{}, err
	}
	active, err := s.Automation().QueryWithDiscarded().Where(automation.StatusEQ(automation.StatusActive)).Count(ctx)
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

// timeSeries buckets the send cohort by UTC send day, then zero-fills every day in
// [since, until). date_trunc is forced to UTC so the labels match the zero-fill loop
// whatever the DB session zone.
func timeSeries(ctx context.Context, s *ent.Scoped, since, until time.Time) ([]Point, error) {
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
				sql.As(fmt.Sprintf("to_char(date_trunc('day', %s AT TIME ZONE 'UTC'), 'YYYY-MM-DD')", sentAt), "day"),
				sql.As("COUNT(*)", "sent"),
				sql.As(fmt.Sprintf("COUNT(%s)", sel.C(broadcastrecipient.FieldOpenedAt)), "opened"),
				sql.As(fmt.Sprintf("COUNT(%s)", sel.C(broadcastrecipient.FieldClickedAt)), "clicked"),
			).
				Where(sql.And(
					sql.EQ(m.C(outboundmessage.FieldStatus), string(outboundmessage.StatusSent)),
					sql.GTE(sentAt, since), sql.LT(sentAt, until),
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
	var points []Point
	for d := since; d.Before(until); d = d.AddDate(0, 0, 1) {
		day := d.Format(dayFormat)
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
