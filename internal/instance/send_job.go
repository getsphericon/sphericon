package instance

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/jobs"
)

// reportCheckInterval is how often the job looks at whether a report is due: a report goes
// out at most once a day, at a random time inside it.
const reportCheckInterval = time.Hour

// SendReportArgs is the tick that sends the Telemetry report when one is due.
type SendReportArgs struct{}

func (SendReportArgs) Kind() string { return "telemetry_report" }

// sendReportWorker runs the sender. A failed send is not a job failure: the next tick tries
// again.
type sendReportWorker struct {
	river.WorkerDefaults[SendReportArgs]
	s *Sender
}

func (w *sendReportWorker) Work(ctx context.Context, _ *river.Job[SendReportArgs]) error {
	return w.s.Run(ctx)
}

// Jobs is the sender's worker and periodic job, handed to the job client as an extension.
func (s *Sender) Jobs() jobs.Extension {
	return jobs.Extension{
		Workers: func(w *river.Workers) { river.AddWorker(w, &sendReportWorker{s: s}) },
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(reportCheckInterval),
				func() (river.JobArgs, *river.InsertOpts) { return SendReportArgs{}, &river.InsertOpts{MaxAttempts: 1} },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	}
}
