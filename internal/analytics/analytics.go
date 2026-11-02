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
//
// Sending counts every message sent in the window, through every surface, from the Outbound
// messages themselves (ADR 0053): what went out and what the provider said became of it.
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

// Overview is everything the dashboard shows for one Workspace and window: the snapshots, which
// have no earlier period, and the figures of the window with those of the period just before it.
type Overview struct {
	Contacts    Contacts
	Automations Automations
	// Current is the window ending today; Previous is as many Report days just before it
	// (reportday.Window.Previous, the rule of the Automation report).
	Current, Previous Period
	// Series is the current period by Report day.
	Series []Point
}

// Period is the figures of one period of Report days.
type Period struct {
	// From and To are its first and last Report day, YYYY-MM-DD.
	From, To    string
	Sending     Sending
	Email       Email
	NewContacts int
}

// Contacts is the contact breakdown now: Unsubscribed is "globally non-mailable" (ADR 0001),
// Active the remainder.
type Contacts struct {
	Total, Active, Unsubscribed int
}

// Sending is the messages sent in the window by the surface that asked for them, and of all of
// them, those a Delivery notice said were delivered, bounced or complained about.
type Sending struct {
	Broadcast, Automation, Transactional int
	Delivered, Bounced, Complained       int
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
	now       func() time.Time
	retention time.Duration
}

func New() *Module { return &Module{now: time.Now} }

// WithClock returns the module reading now from the given clock.
func (m *Module) WithClock(now func() time.Time) *Module {
	c := *m
	c.now = now
	return &c
}

// WithRetention returns the module that reports d as the Event retention (config.EventsRetention):
// the age past which Events are deleted, 0 when they are kept forever.
func (m *Module) WithRetention(d time.Duration) *Module {
	c := *m
	c.retention = d
	return &c
}

// RetentionDays is the Event retention in whole days, which a report over history states next
// to its figures.
func (m *Module) RetentionDays() int { return int(m.retention / (24 * time.Hour)) }

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

// Overview computes the dashboard for the Workspace behind s over the window.
func (m *Module) Overview(ctx context.Context, s *ent.Scoped, w Window) (*Overview, error) {
	days, err := m.days(ctx, s, w)
	if err != nil {
		return nil, err
	}
	contacts, err := contactCounts(ctx, s)
	if err != nil {
		return nil, err
	}
	automations, err := automationCounts(ctx, s)
	if err != nil {
		return nil, err
	}
	current, err := periodOf(ctx, s, days)
	if err != nil {
		return nil, err
	}
	prev := days.Previous()
	previous, err := periodOf(ctx, s, prev)
	if err != nil {
		return nil, err
	}
	series, err := timeSeries(ctx, s, days)
	if err != nil {
		return nil, err
	}
	return &Overview{Contacts: contacts, Automations: automations, Current: current, Previous: previous, Series: series}, nil
}

// periodOf reads the figures of the period of Report days w.
func periodOf(ctx context.Context, s *ent.Scoped, w reportday.Window) (Period, error) {
	sending, err := sendingCounts(ctx, s, w.Since, w.Until)
	if err != nil {
		return Period{}, err
	}
	email, err := emailCounts(ctx, s, w.Since, w.Until)
	if err != nil {
		return Period{}, err
	}
	newContacts, err := s.Contact().Query().Where(contact.CreatedAtGTE(w.Since), contact.CreatedAtLT(w.Until)).Count(ctx)
	if err != nil {
		return Period{}, err
	}
	return Period{From: w.First(), To: w.Last(), Sending: sending, Email: email, NewContacts: newContacts}, nil
}

func contactCounts(ctx context.Context, s *ent.Scoped) (Contacts, error) {
	total, err := s.Contact().Query().Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	unsub, err := s.Contact().Query().Where(eligibility.GloballyOptedOut(eligibility.ChannelEmail)).Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	return Contacts{Total: total, Active: total - unsub, Unsubscribed: unsub}, nil
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

// sendingCounts counts the Outbound messages sent in [since, until) per kind, and the delivery
// stamps among them.
func sendingCounts(ctx context.Context, s *ent.Scoped, since, until time.Time) (Sending, error) {
	var rows []struct {
		Kind       outboundmessage.Kind `sql:"kind"`
		Sent       int                  `sql:"sent"`
		Delivered  int                  `sql:"delivered"`
		Bounced    int                  `sql:"bounced"`
		Complained int                  `sql:"complained"`
	}
	err := s.OutboundMessage().Query().
		Where(
			outboundmessage.StatusEQ(outboundmessage.StatusSent),
			outboundmessage.SentAtGTE(since),
			outboundmessage.SentAtLT(until),
		).
		Modify(func(sel *sql.Selector) {
			sel.Select(
				sel.C(outboundmessage.FieldKind),
				sql.As(sql.Count("*"), "sent"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldDeliveredAt)), "delivered"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldBouncedAt)), "bounced"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldComplainedAt)), "complained"),
			).GroupBy(sel.C(outboundmessage.FieldKind))
		}).
		Scan(ctx, &rows)
	if err != nil {
		return Sending{}, err
	}
	var out Sending
	for _, r := range rows {
		switch r.Kind {
		case outboundmessage.KindBroadcast:
			out.Broadcast = r.Sent
		case outboundmessage.KindAutomation:
			out.Automation = r.Sent
		case outboundmessage.KindTransactional:
			out.Transactional = r.Sent
		}
		out.Delivered += r.Delivered
		out.Bounced += r.Bounced
		out.Complained += r.Complained
	}
	return out, nil
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
			sel.AppendSelectExprAs(reportday.Label(sentAt, days.Loc), "day").
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
