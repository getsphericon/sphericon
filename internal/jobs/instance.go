package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
)

// The Instance's own ticks: the Release check and the Telemetry report. The Instance state module
// does the work; the jobs and their schedules are the runner's.

// ReleaseCheckArgs is the daily tick of the Release check. A restart checks again at once, which
// is harmless: the check sends nothing.
type ReleaseCheckArgs struct{}

func (ReleaseCheckArgs) Kind() string { return "release_check" }

// ReleaseCheckWorker runs the Release check (instancestates.Module.CheckRelease).
type ReleaseCheckWorker struct {
	river.WorkerDefaults[ReleaseCheckArgs]

	instance *instancestates.Module
}

// NewReleaseCheckWorker is the Release check's worker.
func NewReleaseCheckWorker(instance *instancestates.Module) *ReleaseCheckWorker {
	return &ReleaseCheckWorker{instance: instance}
}

func (w *ReleaseCheckWorker) Work(ctx context.Context, _ *river.Job[ReleaseCheckArgs]) error {
	return w.instance.CheckRelease(ctx)
}

// telemetryReportCheckInterval is how often the job looks at whether a Telemetry report is due: a
// report goes out at most once a day, at a random time inside it.
const telemetryReportCheckInterval = time.Hour

// TelemetryReportArgs is the tick that sends the Telemetry report when one is due.
type TelemetryReportArgs struct{}

func (TelemetryReportArgs) Kind() string { return "telemetry_report" }

// TelemetryReportWorker sends the report when one is due
// (instancestates.Module.SendTelemetryReport). A failed send is not a job failure: the next tick
// tries again.
type TelemetryReportWorker struct {
	river.WorkerDefaults[TelemetryReportArgs]

	instance *instancestates.Module
}

// NewTelemetryReportWorker is the Telemetry report's worker.
func NewTelemetryReportWorker(instance *instancestates.Module) *TelemetryReportWorker {
	return &TelemetryReportWorker{instance: instance}
}

func (w *TelemetryReportWorker) Work(ctx context.Context, _ *river.Job[TelemetryReportArgs]) error {
	return w.instance.SendTelemetryReport(ctx)
}

// instanceJobs adds the workers of the Instance's ticks when deps has the Instance state, and
// returns their schedules.
func instanceJobs(workers *river.Workers, deps Deps) []*river.PeriodicJob {
	if deps.Instance == nil {
		return nil
	}
	river.AddWorker(workers, NewReleaseCheckWorker(deps.Instance))
	river.AddWorker(workers, NewTelemetryReportWorker(deps.Instance))
	return []*river.PeriodicJob{
		river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) { return ReleaseCheckArgs{}, nil },
			&river.PeriodicJobOpts{RunOnStart: true}),
		river.NewPeriodicJob(river.PeriodicInterval(telemetryReportCheckInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return TelemetryReportArgs{}, &river.InsertOpts{MaxAttempts: 1}
			},
			&river.PeriodicJobOpts{RunOnStart: true}),
	}
}
