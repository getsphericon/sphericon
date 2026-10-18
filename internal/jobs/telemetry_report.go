package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/instance"
)

// telemetryReportInterval is how often the sender looks at whether a Telemetry report is
// due: it sends at most once a day, at a random time inside it (ADR 0032).
const telemetryReportInterval = time.Hour

// SendTelemetryReportArgs is the tick that sends the Telemetry report when one is due.
type SendTelemetryReportArgs struct{}

func (SendTelemetryReportArgs) Kind() string { return "telemetry_report" }

// SendTelemetryReportWorker runs the Instance's report sender. A failed send is not a job
// failure: the next tick tries again.
type SendTelemetryReportWorker struct {
	river.WorkerDefaults[SendTelemetryReportArgs]
	sender *instance.Sender
}

func (w *SendTelemetryReportWorker) Work(ctx context.Context, _ *river.Job[SendTelemetryReportArgs]) error {
	return w.sender.Run(ctx)
}

// TelemetryReportExtension is the sender's worker and periodic job, plugged in beside the
// Edition's.
func TelemetryReportExtension(sender *instance.Sender) Extension {
	return Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, &SendTelemetryReportWorker{sender: sender})
		},
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(telemetryReportInterval),
				func() (river.JobArgs, *river.InsertOpts) {
					return SendTelemetryReportArgs{}, &river.InsertOpts{MaxAttempts: 1}
				},
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	}
}
