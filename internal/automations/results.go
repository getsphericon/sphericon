package automations

import (
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ReportPeriod is the length of the period an Automation's results are read over, in Report days
// (ADR 0057), today included.
type ReportPeriod int

const (
	ReportPeriod7Days  ReportPeriod = 7
	ReportPeriod14Days ReportPeriod = 14
	ReportPeriod30Days ReportPeriod = 30
	ReportPeriod90Days ReportPeriod = 90
)

var reportPeriods = []ReportPeriod{ReportPeriod7Days, ReportPeriod14Days, ReportPeriod30Days, ReportPeriod90Days}

// ErrUnknownPeriod is a period that is not one of the report's.
var ErrUnknownPeriod = validation.RefuseField("period", "automations: unknown report period", i18n.ErrorsAutomationUnknownReportPeriod)

// PeriodOf is the ReportPeriod a period of the contract names, written as its number of days and
// "d" (`7d`): the one mapping both surfaces use.
func PeriodOf[P ~string](p P) (ReportPeriod, error) {
	n, ok := strings.CutSuffix(string(p), "d")
	days, err := strconv.Atoi(n)
	period := ReportPeriod(days)
	if !ok || err != nil || !slices.Contains(reportPeriods, period) {
		return 0, ErrUnknownPeriod
	}
	return period, nil
}

// Results is what an Automation's Enrollments did over a period of Report days and over the
// period just before it (#753), read from the Enrollments themselves. Each figure counts what
// happened in the period: an Enrollment is entered on the Report day it began (began_at, when
// its triggering Event occurred) and counts under its outcome on the Report day it ended
// (ended_at). So a period is not short of the outcomes its newest Enrollments have not reached
// yet, and two periods compare like for like.
type Results struct {
	Current, Previous PeriodResults
	// Series is the current period by Report day, oldest first, a day with nothing at zero.
	Series []ResultsPoint
}

// ResultCounts is how many Enrollments began, and how many ended with each outcome.
type ResultCounts struct {
	Entered, Completed, GoalReached, Exited, Failed int
}

// PeriodResults is one period of Results.
type PeriodResults struct {
	ResultCounts

	// From and To are its first and last Report day, YYYY-MM-DD.
	From, To string
	// AverageDuration is the mean time from beginning to end of the Enrollments that ended in the
	// period, whatever their outcome; nil when none did. An active Enrollment has no duration yet.
	AverageDuration *time.Duration
	// Revenue is what the Automation earned in the period (ADR 0064): the Orders credited to it
	// that were completed then, and the net revenue of the Movements of its Orders then.
	Revenue orders.Revenue
}

// ResultsPoint is one Report day of Results.
type ResultsPoint struct {
	ResultCounts

	Date string
}

func (c *ResultCounts) add(status automationrun.Status, n int) {
	switch status {
	case automationrun.StatusCompleted:
		c.Completed += n
	case automationrun.StatusGoalReached:
		c.GoalReached += n
	case automationrun.StatusExited:
		c.Exited += n
	case automationrun.StatusFailed:
		c.Failed += n
	case automationrun.StatusActive:
		// An active Enrollment has not ended, so it has no ended_at to be counted by.
	}
}

func (c *ResultCounts) plus(o ResultCounts) {
	c.Entered += o.Entered
	c.Completed += o.Completed
	c.GoalReached += o.GoalReached
	c.Exited += o.Exited
	c.Failed += o.Failed
}

// resultsOf reads the Results of the Automation id over the period ending today in the
// Workspace time zone.
func (m *Module) resultsOf(ctx context.Context, s *ent.Scoped, id int64, p ReportPeriod) (Results, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return Results{}, err
	}
	loc, err := reportday.Location(ws.Timezone)
	if err != nil {
		return Results{}, err
	}
	now := m.now()
	cur := reportday.Trailing(now, loc, int(p))
	prev := cur.Previous()
	// Twice the period ending today is both periods: the previous one and the current one.
	byDay, err := countsByDay(ctx, s, id, reportday.Trailing(now, loc, 2*int(p)))
	if err != nil {
		return Results{}, err
	}
	current, err := periodResultsOf(ctx, s, id, cur, byDay)
	if err != nil {
		return Results{}, err
	}
	previous, err := periodResultsOf(ctx, s, id, prev, byDay)
	if err != nil {
		return Results{}, err
	}
	series := make([]ResultsPoint, 0, len(cur.Dates))
	for _, day := range cur.Dates {
		series = append(series, ResultsPoint{Date: day, ResultCounts: byDay[day]})
	}
	return Results{Current: current, Previous: previous, Series: series}, nil
}

// countsByDay counts the Automation's Enrollments that began, and those that ended by outcome, in
// the window, by the Report day of the moment each happened.
func countsByDay(ctx context.Context, s *ent.Scoped, id int64, w reportday.Window) (map[string]ResultCounts, error) {
	byDay := map[string]ResultCounts{}
	var began []struct {
		Day string `sql:"day"`
		N   int    `sql:"n"`
	}
	err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(id), automationrun.BeganAtGTE(w.Since), automationrun.BeganAtLT(w.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(sql.As(sql.Count("*"), "n")).
				AppendSelectExprAs(reportday.Label(sel.C(automationrun.FieldBeganAt), w.Loc), "day").
				GroupBy("day")
		}).Scan(ctx, &began)
	if err != nil {
		return nil, err
	}
	for _, r := range began {
		c := byDay[r.Day]
		c.Entered = r.N
		byDay[r.Day] = c
	}
	var ended []struct {
		Day    string `sql:"day"`
		Status string `sql:"status"`
		N      int    `sql:"n"`
	}
	err = s.AutomationRun().Query().
		Where(automationrun.AutomationID(id), automationrun.EndedAtGTE(w.Since), automationrun.EndedAtLT(w.Until)).
		Modify(func(sel *sql.Selector) {
			sel.Select(sel.C(automationrun.FieldStatus), sql.As(sql.Count("*"), "n")).
				AppendSelectExprAs(reportday.Label(sel.C(automationrun.FieldEndedAt), w.Loc), "day").
				GroupBy(sel.C(automationrun.FieldStatus), "day")
		}).Scan(ctx, &ended)
	if err != nil {
		return nil, err
	}
	for _, r := range ended {
		c := byDay[r.Day]
		c.add(automationrun.Status(r.Status), r.N)
		byDay[r.Day] = c
	}
	return byDay, nil
}

// periodResultsOf sums the days of w and reads its average duration and revenue.
func periodResultsOf(ctx context.Context, s *ent.Scoped, id int64, w reportday.Window, byDay map[string]ResultCounts) (PeriodResults, error) {
	out := PeriodResults{From: w.Dates[0], To: w.Dates[len(w.Dates)-1]}
	for _, day := range w.Dates {
		out.plus(byDay[day])
	}
	var avg []struct {
		Seconds *float64 `sql:"seconds"`
	}
	err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(id), automationrun.EndedAtGTE(w.Since), automationrun.EndedAtLT(w.Until)).
		Modify(func(sel *sql.Selector) {
			// raw sql: ent has no interval arithmetic; the mean of ended_at - began_at in seconds.
			// The clamp at zero is for an Enrollment a step or an unsubscribe ended by the clock:
			// it began when its triggering Event occurred, and a live Event may be dated ahead of
			// the server's clock (eventlog refuses only a Backfill from the future), so the clock
			// can end it before it began. An exit or goal Event ends it at that Event's time, which
			// is never before it began.
			sel.Select(sql.As("AVG(GREATEST(EXTRACT(EPOCH FROM "+sel.C(automationrun.FieldEndedAt)+" - "+sel.C(automationrun.FieldBeganAt)+"), 0))::float8", "seconds"))
		}).Scan(ctx, &avg)
	if err != nil {
		return PeriodResults{}, err
	}
	if len(avg) > 0 && avg[0].Seconds != nil {
		d := time.Duration(math.Round(*avg[0].Seconds)) * time.Second
		out.AverageDuration = &d
	}
	revenue, err := orders.CreditedToAutomationsBetween(ctx, s, w.Since, w.Until, id)
	if err != nil {
		return PeriodResults{}, err
	}
	out.Revenue = revenue.Of(id)
	return out, nil
}
