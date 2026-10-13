// Package usage materializes what each Workspace did per Usage period (ADR 0009): the
// Usage snapshot of a closed period and the Live usage of the open one, one row per
// (Workspace, period, metric) in the usages table. One periodic river job, registered
// through Edition.Jobs() like the retention prune, closes the elapsed periods and
// refreshes the open one; a read never recounts. Unlicensed (no `operator` feature),
// the job does nothing and no row is ever written.
//
// The raw client is used only to list Workspaces and to page their figures; every
// measurement and write goes through the Workspace's scoped client.
//
// Adding a metric: declare a Metric constant, add a measurer to metrics (and the
// UsageMetric enum of typespec/service/resources/usage.tsp). A measurer gets the
// period, whether it is still open and the prior row, so a high-water mark can keep the
// peak it already stored. Nothing else changes: the schema holds the metric as a string.
//
// Governed by ee/LICENSE, not the AGPL.
package usage

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Interval is how often the job closes elapsed periods and refreshes Live usage.
const Interval = 5 * time.Minute

// Metric names one measured figure.
type Metric string

// EmailsSent is the number of emails sent: one per recipient across Broadcast,
// Automation and Transactional, never a suppressed, held or failed message.
const EmailsSent Metric = "emails_sent"

// InFlightSends is the number of sends claimed and not yet answered. It is Live usage
// only, kept apart from EmailsSent so an unanswered send is never billed as sent; it is 0
// in a Usage snapshot.
const InFlightSends Metric = "in_flight_sends"

// Period is one Usage period: the calendar month in UTC, [Start, End).
type Period struct{ Start, End time.Time }

// PeriodOf is the Usage period containing t.
func PeriodOf(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func periodFrom(start time.Time) Period { return Period{Start: start, End: start.AddDate(0, 1, 0)} }

// measurer computes one metric of a Workspace over a period. open is true for the
// period still running; prior is the
// row already stored for it, or nil.
type measurer func(ctx context.Context, s *ent.Scoped, p Period, open bool, prior *ent.Usage) (int64, error)

var metrics = []struct {
	metric  Metric
	measure measurer
}{
	{EmailsSent, measureEmailsSent},
	{InFlightSends, measureInFlightSends},
	{Contacts, measureContacts},
}

// measureEmailsSent counts the Outbound messages the provider accepted in the period,
// by the time they were accepted: a send accepted after the period ended belongs to the
// next one, so the figure of a closed period is the same whenever it is computed. Sends
// still in flight are never in it: they are the separate InFlightSends figure.
func measureEmailsSent(ctx context.Context, s *ent.Scoped, p Period, _ bool, _ *ent.Usage) (int64, error) {
	n, err := s.OutboundMessage().Query().Where(
		outboundmessage.StatusEQ(outboundmessage.StatusSent),
		outboundmessage.SentAtGTE(p.Start), outboundmessage.SentAtLT(p.End),
	).Count(ctx)
	return int64(n), err
}

// measureInFlightSends counts the sends claimed in the open period and not yet answered.
// A closed period has none: what was still in flight at the close is counted by
// EmailsSent in the period it is accepted in.
func measureInFlightSends(ctx context.Context, s *ent.Scoped, p Period, open bool, _ *ent.Usage) (int64, error) {
	if !open {
		return 0, nil
	}
	n, err := s.OutboundMessage().Query().Where(
		outboundmessage.StatusEQ(outboundmessage.StatusPending),
		outboundmessage.ClaimedAtGTE(p.Start), outboundmessage.ClaimedAtLT(p.End),
	).Count(ctx)
	return int64(n), err
}

// Refresh closes every elapsed Usage period that is not closed yet and refreshes the
// Live usage of the open one, for every Workspace, as of now. It is a pure function of
// now and the stored data: running it twice or late gives the same rows, and a closed
// figure is never rewritten. Without the operator license it does nothing.
func Refresh(ctx context.Context, client *ent.Client, lic *licensekey.License, now time.Time) error {
	if !lic.Has(licensekey.FeatureOperator) {
		return nil
	}
	workspaces, err := client.Workspace.Query().
		Select(workspace.FieldID, workspace.FieldCreatedAt).
		Order(ent.Asc(workspace.FieldID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	current := PeriodOf(now)
	for _, ws := range workspaces {
		if err := refreshWorkspace(ctx, client.Scoped(ws.ID), PeriodOf(ws.CreatedAt), current, now); err != nil {
			return fmt.Errorf("usage of workspace %d: %w", ws.ID, err)
		}
	}
	return nil
}

func refreshWorkspace(ctx context.Context, s *ent.Scoped, first, current, now time.Time) error {
	rows, err := s.Usage().Query().Where(entusage.PeriodStartGTE(first), entusage.PeriodStartLTE(current)).All(ctx)
	if err != nil {
		return err
	}
	type key struct {
		start  time.Time
		metric Metric
	}
	stored := make(map[key]*ent.Usage, len(rows))
	for _, r := range rows {
		stored[key{r.PeriodStart.UTC(), Metric(r.Metric)}] = r
	}
	for start := first; !start.After(current); start = start.AddDate(0, 1, 0) {
		open := start.Equal(current)
		for _, m := range metrics {
			prior := stored[key{start, m.metric}]
			if prior != nil && prior.Final {
				continue
			}
			if err := materialize(ctx, s, periodFrom(start), m.metric, m.measure, open, prior, now); err != nil {
				return fmt.Errorf("%s of %s: %w", m.metric, start.Format("2006-01"), err)
			}
		}
	}
	return nil
}

// materialize measures one metric and stores it: Live usage while the period is open,
// the Usage snapshot (final) once it has closed.
func materialize(ctx context.Context, s *ent.Scoped, p Period, metric Metric, measure measurer, open bool, prior *ent.Usage, now time.Time) error {
	value, err := measure(ctx, s, p, open, prior)
	if err != nil {
		return err
	}
	if prior == nil {
		return s.Usage().Create().
			SetPeriodStart(p.Start).SetMetric(string(metric)).
			SetValue(value).SetFinal(!open).SetComputedAt(now).
			Exec(ctx)
	}
	return s.Usage().UpdateOneID(prior.ID).
		Where(entusage.Final(false)).
		SetValue(value).SetFinal(!open).SetComputedAt(now).
		Exec(ctx)
}

// Figure is one metric's value over a period.
type Figure struct {
	Metric     Metric
	Value      int64
	Final      bool
	ComputedAt time.Time
}

// WorkspaceUsage is the figures of one Workspace for one period.
type WorkspaceUsage struct {
	WorkspaceID int64
	Period      time.Time
	Figures     []Figure
}

// List is one page of the stored figures of every Workspace that has any for the
// period, by Workspace id. It reads the stored rows and never recounts.
func List(ctx context.Context, client *ent.Client, period time.Time, p pagination.Params) (pagination.Page[WorkspaceUsage], error) {
	period = PeriodOf(period)
	distinct := func() *ent.UsageQuery {
		return client.Usage.Query().Where(entusage.PeriodStart(period)).Unique(true)
	}
	return pagination.List(ctx, p,
		distinct().Select(entusage.FieldWorkspaceID).Count,
		func(ctx context.Context, limit, offset int) ([]WorkspaceUsage, error) {
			var ids []int64
			if err := distinct().Order(ent.Asc(entusage.FieldWorkspaceID)).Limit(limit).Offset(offset).
				Select(entusage.FieldWorkspaceID).Scan(ctx, &ids); err != nil {
				return nil, err
			}
			rows, err := client.Usage.Query().
				Where(entusage.PeriodStart(period), entusage.WorkspaceIDIn(ids...)).
				Order(ent.Asc(entusage.FieldWorkspaceID), ent.Asc(entusage.FieldMetric)).
				All(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]WorkspaceUsage, 0, len(ids))
			for _, r := range rows {
				if n := len(out); n == 0 || out[n-1].WorkspaceID != r.WorkspaceID {
					out = append(out, WorkspaceUsage{WorkspaceID: r.WorkspaceID, Period: period})
				}
				w := &out[len(out)-1]
				w.Figures = append(w.Figures, Figure{Metric: Metric(r.Metric), Value: r.Value, Final: r.Final, ComputedAt: r.ComputedAt})
			}
			return out, nil
		})
}

// Get is the stored figures of one Workspace for the period, by metric. It reads the
// stored rows and never recounts; a Workspace with none yet has no figures.
func Get(ctx context.Context, s *ent.Scoped, period time.Time) (WorkspaceUsage, error) {
	period = PeriodOf(period)
	rows, err := s.Usage().Query().
		Where(entusage.PeriodStart(period)).
		Order(ent.Asc(entusage.FieldMetric)).
		All(ctx)
	if err != nil {
		return WorkspaceUsage{}, err
	}
	out := WorkspaceUsage{WorkspaceID: s.WorkspaceID(), Period: period}
	for _, r := range rows {
		out.Figures = append(out.Figures, Figure{Metric: Metric(r.Metric), Value: r.Value, Final: r.Final, ComputedAt: r.ComputedAt})
	}
	return out, nil
}

// Args is the periodic usage job.
type Args struct{}

func (Args) Kind() string { return "ee_usage_refresh" }

// Worker runs Refresh on the periodic tick.
type Worker struct {
	river.WorkerDefaults[Args]
	client *ent.Client
	lic    *licensekey.License
}

// NewWorker builds the usage worker. It is a raw-client entry like the retention job:
// it lists Workspaces, then scopes to each one.
func NewWorker(client *ent.Client, lic *licensekey.License) *Worker {
	return &Worker{client: client, lic: lic}
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[Args]) error {
	return Refresh(ctx, w.client, w.lic, time.Now())
}

// Periodic is the river schedule of the usage job.
func Periodic() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(Interval),
		func() (river.JobArgs, *river.InsertOpts) { return Args{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}
