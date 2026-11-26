// Package analyticsreport is the use case that reads the Analytics reports (ADR 0102): the
// dashboard's Overview (contacts, Engagement rates, automations and the daily time series,
// GLOSSARY "Engagement rates") and the reports beside it. It reads the rows of the modules that
// hold the facts and writes none.
//
// Days are Report days (ADR 0057): calendar days in the Workspace time zone, cut by
// internal/kernel/reportday, so the whole history is re-bucketed the moment the zone changes.
//
// Engagement rates are a send cohort: the denominator is the tracked Broadcast and Automation
// messages sent in the window (ADR 0120), and opens and clicks are counted among that same cohort,
// so opened never exceeds sent and every rate is within [0, 1]. The messages that are not tracked
// (sent untracked, or withdrawn since) are counted beside the rates, never in them. They are not
// the Sending-domain Complaint and Bounce rates of internal/domain/sending/reputation.
//
// Sending counts every message sent in the window, through every surface, from the Outbound
// messages themselves (ADR 0053): what went out and what the provider said became of it.
package analyticsreport

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
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
	// Deliverability is the Sending domains' rates over the trailing 24 hours; the window does not
	// follow the range.
	Deliverability Deliverability
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

// Email is the send cohort's counts and Engagement rates: Sent, Opened and Clicked are the tracked
// messages', Untracked the Broadcast and Automation messages sent in the window that are not tracked.
type Email struct {
	Sent, Opened, Clicked, Untracked     int
	OpenRate, ClickRate, ClickToOpenRate float32
}

// Automations is a point-in-time snapshot.
type Automations struct {
	Total, Active, RunsActive, RunsCompleted int
}

// Point is one Report day of the send cohort, and of the untracked messages beside it.
type Point struct {
	Date                             string
	Sent, Opened, Clicked, Untracked int
}

// UseCase computes Overviews.
type UseCase struct {
	clicks    *linkclick.Module
	now       func() time.Time
	retention time.Duration
	revenues  *orders.Revenues
}

// New builds the use case over the count of the Link clicks and the Instance clock, with retention the Event retention it reports
// (config.EventsRetention): the age past which Events are deleted, 0 when they are kept forever.
func New(clicks *linkclick.Module, now func() time.Time, retention time.Duration) *UseCase {
	return &UseCase{clicks: clicks, now: now, retention: retention, revenues: orders.NewRevenues()}
}

// RetentionDays is the Event retention in whole days, which a report over history states next
// to its figures.
func (m *UseCase) RetentionDays() int { return int(m.retention / (24 * time.Hour)) }

// days is the trailing Report-day window of w days for the Workspace behind s, read in its
// time zone; every report of the use case cuts its window here.
func (m *UseCase) days(ctx context.Context, s *ent.Scoped, w Window) (reportday.Window, error) {
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
func (m *UseCase) Overview(ctx context.Context, s *ent.Scoped, w Window) (*Overview, error) {
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
	deliverability, err := m.overviewDeliverability(ctx, s)
	if err != nil {
		return nil, err
	}
	return &Overview{Contacts: contacts, Automations: automations, Current: current, Previous: previous, Series: series, Deliverability: deliverability}, nil
}

// periodOf reads the figures of the period of Report days w.
func periodOf(ctx context.Context, s *ent.Scoped, w reportday.Window) (Period, error) {
	sending, email, err := sendCounts(ctx, s, w.Since, w.Until)
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
	unsub, err := s.Contact().Query().Where(eligibility.GloballyOptedOut(channels.Email)).Count(ctx)
	if err != nil {
		return Contacts{}, err
	}
	return Contacts{Total: total, Active: total - unsub, Unsubscribed: unsub}, nil
}

// marketingSent matches the marketing messages sent in [since, until): the Broadcast and
// Automation sends, a Transactional one left out (ADR 0053). The send time is the Outbound
// message's (ADR 0015), and so are its opened and clicked stamps (ADR 0053), so every message
// counts, not one per person.
func marketingSent(since, until time.Time) predicate.OutboundMessage {
	return outboundmessage.And(
		outboundmessage.KindIn(outboundmessage.KindBroadcast, outboundmessage.KindAutomation),
		outboundmessage.StatusEQ(outboundmessage.StatusSent),
		outboundmessage.SentAtGTE(since), outboundmessage.SentAtLT(until),
	)
}

// engagementCohort matches the send cohort of [since, until): the marketing messages sent in it
// that are tracked (ADR 0120). One sent untracked or withdrawn since is out of it, its stamps
// included: a withdrawal by the Workspace's default keeps them.
func engagementCohort(since, until time.Time) predicate.OutboundMessage {
	return outboundmessage.And(marketingSent(since, until), engagementtracking.TrackedMessages())
}

// untrackedBeside matches the marketing messages sent in [since, until) that are not tracked: the
// ones the reports count beside the rates.
func untrackedBeside(since, until time.Time) predicate.OutboundMessage {
	return outboundmessage.And(marketingSent(since, until), engagementtracking.UntrackedMessages())
}

// sendCounts counts the Outbound messages sent in [since, until) per kind and the delivery stamps
// among them, then the engagementCohort's sends, opens and clicks, which are the Engagement rates,
// and the untracked marketing messages beside it.
func sendCounts(ctx context.Context, s *ent.Scoped, since, until time.Time) (Sending, Email, error) {
	sending, err := sendingCounts(ctx, s, since, until)
	if err != nil {
		return Sending{}, Email{}, err
	}
	var cohort []struct {
		Sent    int `sql:"sent"`
		Opened  int `sql:"opened"`
		Clicked int `sql:"clicked"`
	}
	err = s.OutboundMessage().Query().
		Where(engagementCohort(since, until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(
				sql.As(sql.Count("*"), "sent"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldOpenedAt)), "opened"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldClickedAt)), "clicked"),
			)
		}).
		Scan(ctx, &cohort)
	if err != nil {
		return Sending{}, Email{}, err
	}
	untracked, err := s.OutboundMessage().Query().Where(untrackedBeside(since, until)).Count(ctx)
	if err != nil {
		return Sending{}, Email{}, err
	}
	var email Email
	for _, r := range cohort {
		email.Sent, email.Opened, email.Clicked = r.Sent, r.Opened, r.Clicked
	}
	email.Untracked = untracked
	email.OpenRate = broadcasts.Ratio(email.Opened, email.Sent)
	email.ClickRate = broadcasts.Ratio(email.Clicked, email.Sent)
	email.ClickToOpenRate = broadcasts.Ratio(email.Clicked, email.Opened)
	return sending, email, nil
}

// sendingCounts counts the Outbound messages sent in [since, until) per kind, tracked or not, and
// the delivery stamps among them: delivery is not tracking.
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
		case outboundmessage.KindConfirmation:
			// The confirmation emails of double opt-in (ADR 0013) are no surface of the Workspace's
			// own: they count only in what became of all the messages.
		}
		out.Delivered += r.Delivered
		out.Bounced += r.Bounced
		out.Complained += r.Complained
	}
	return out, nil
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

// timeSeries buckets the send cohort, and the untracked messages beside it, by the Report day of
// their send time, then zero-fills every day of the window from the local dates, so a 23- or
// 25-hour day is one bucket.
func timeSeries(ctx context.Context, s *ent.Scoped, days reportday.Window) ([]Point, error) {
	var rows []struct {
		Day     string `sql:"day"`
		Sent    int    `sql:"sent"`
		Opened  int    `sql:"opened"`
		Clicked int    `sql:"clicked"`
	}
	err := s.OutboundMessage().Query().
		Where(engagementCohort(days.Since, days.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(
				sql.As(sql.Count("*"), "sent"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldOpenedAt)), "opened"),
				sql.As(sql.Count(sel.C(outboundmessage.FieldClickedAt)), "clicked"),
			)
			sel.AppendSelectExprAs(reportday.Label(sel.C(outboundmessage.FieldSentAt), days.Loc), "day").
				GroupBy("day")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	var untracked []struct {
		Day   string `sql:"day"`
		Count int    `sql:"untracked"`
	}
	err = s.OutboundMessage().Query().
		Where(untrackedBeside(days.Since, days.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(sql.As(sql.Count("*"), "untracked"))
			sel.AppendSelectExprAs(reportday.Label(sel.C(outboundmessage.FieldSentAt), days.Loc), "day").
				GroupBy("day")
		}).
		Scan(ctx, &untracked)
	if err != nil {
		return nil, err
	}
	byDay := make(map[string]Point, len(rows))
	for _, r := range rows {
		byDay[r.Day] = Point{Date: r.Day, Sent: r.Sent, Opened: r.Opened, Clicked: r.Clicked}
	}
	for _, r := range untracked {
		p := byDay[r.Day]
		p.Untracked = r.Count
		byDay[r.Day] = p
	}
	return zeroFilled(days, byDay, func(p *Point, day string) { p.Date = day }), nil
}

// zeroFilled lists one point per Report day of the window, in order: the day's point of byDay,
// or the zero point on a day with none, each dated by date.
func zeroFilled[P any](days reportday.Window, byDay map[string]P, date func(*P, string)) []P {
	points := make([]P, 0, len(days.Dates))
	for _, day := range days.Dates {
		p := byDay[day]
		date(&p, day)
		points = append(points, p)
	}
	return points
}
