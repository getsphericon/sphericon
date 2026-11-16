// Package jobs is the Enterprise Edition's job runner (ADR 0111): the workers and periodic
// schedules of its modules and use cases, and its consumers of the domain events. It is a driver
// with the core runner's rules (review rule 5): each worker receives a job and calls one operation of
// an Enterprise module or use case, which checks the License itself. Only the wiring builds it, and
// hands its Extension to the core's job client.
//
// Governed by ee/LICENSE, not the AGPL.
package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/domain/insights/erasures"
	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// EventConsumers are the Edition's consumers of the domain events (ADR 0101): the Audit log's and
// the Erasures stream's, for the composition root to register on the bus beside core's. They want
// what they want whatever the license: each job checks the license itself, so a key added later
// needs no rewiring and an unlicensed instance stores nothing.
func EventConsumers() []events.Consumer {
	return []events.Consumer{
		{
			Wants: func(env events.Envelope, _ events.DomainEvent) bool { return env.Name == events.NameAuditEntry },
			Job:   events.EnvelopeJob[EntryEventArgs],
		},
		{
			Wants: func(env events.Envelope, _ events.DomainEvent) bool { return env.Name == systemevent.ContactErased },
			Job:   events.EnvelopeJob[ErasureEventArgs],
		},
	}
}

// EntryEventArgs is the Audit log's job: one Audit entry to store (ADR 0022).
type EntryEventArgs struct {
	events.EventJob
}

func (EntryEventArgs) Kind() string { return "event_audit" }

// EntryEventWorker stores the Audit entry of its event (auditentries.Log.Record).
type EntryEventWorker struct {
	river.WorkerDefaults[EntryEventArgs]

	log *auditentries.Log
}

// NewEntryEventWorker builds the Audit log's consumer.
func NewEntryEventWorker(log *auditentries.Log) *EntryEventWorker {
	return &EntryEventWorker{log: log}
}

func (w *EntryEventWorker) Work(ctx context.Context, job *river.Job[EntryEventArgs]) error {
	return w.log.Record(ctx, job.Args.Envelope)
}

// ErasureEventArgs is the Erasures stream's job: one Erasure to add to the stream (ADR 0045).
type ErasureEventArgs struct {
	events.EventJob
}

func (ErasureEventArgs) Kind() string { return "event_export_erasure" }

// ErasureEventWorker adds the Erasure of its event to the stream (erasures.Module.Record).
type ErasureEventWorker struct {
	river.WorkerDefaults[ErasureEventArgs]

	erasures *erasures.Module
}

// NewErasureEventWorker builds the Erasures stream's consumer.
func NewErasureEventWorker(stream *erasures.Module) *ErasureEventWorker {
	return &ErasureEventWorker{erasures: stream}
}

func (w *ErasureEventWorker) Work(ctx context.Context, job *river.Job[ErasureEventArgs]) error {
	return w.erasures.Record(ctx, job.Args.Envelope)
}

// RetentionPruneArgs is the advanced-retention prune (ADR 0014) and the clearing of the Audit
// entries' client details past auditlogretention.ClientWindow (ADR 0050), every
// auditlogretention.Interval.
type RetentionPruneArgs struct{}

func (RetentionPruneArgs) Kind() string { return "ee_retention_prune" }

type retentionPruneWorker struct {
	river.WorkerDefaults[RetentionPruneArgs]

	retention *auditlogretention.UseCase
}

func (w *retentionPruneWorker) Work(ctx context.Context, _ *river.Job[RetentionPruneArgs]) error {
	return w.retention.Sweep(ctx)
}

// UsageRefreshArgs closes the elapsed Usage periods and refreshes the open one (ADR 0009), every
// usages.Interval.
type UsageRefreshArgs struct{}

func (UsageRefreshArgs) Kind() string { return "ee_usage_refresh" }

type usageRefreshWorker struct {
	river.WorkerDefaults[UsageRefreshArgs]

	usage *usages.Module
}

func (w *usageRefreshWorker) Work(ctx context.Context, _ *river.Job[UsageRefreshArgs]) error {
	return w.usage.Refresh(ctx)
}

// AbuseDetectionArgs is a pass of the abuse detector (ADR 0026), every abusedetector.Interval.
type AbuseDetectionArgs struct{}

func (AbuseDetectionArgs) Kind() string { return "ee_abuse_detector" }

type abuseDetectionWorker struct {
	river.WorkerDefaults[AbuseDetectionArgs]

	detector *abusedetector.Detector
}

func (w *abuseDetectionWorker) Work(ctx context.Context, _ *river.Job[AbuseDetectionArgs]) error {
	_, err := w.detector.Run(ctx)
	return err
}

// ExportPruneArgs prunes the Erasures stream past the instance's Event retention window (ADR 0019,
// ADR 0045), every erasures.Interval.
type ExportPruneArgs struct{}

func (ExportPruneArgs) Kind() string { return "ee_export_prune" }

type exportPruneWorker struct {
	river.WorkerDefaults[ExportPruneArgs]

	erasures  *erasures.Module
	retention time.Duration
}

func (w *exportPruneWorker) Work(ctx context.Context, _ *river.Job[ExportPruneArgs]) error {
	_, err := w.erasures.Prune(ctx, w.retention)
	return err
}

// every is the periodic schedule of args every interval, which also runs at start.
func every(interval time.Duration, args river.JobArgs) *river.PeriodicJob {
	return river.NewPeriodicJob(river.PeriodicInterval(interval),
		func() (river.JobArgs, *river.InsertOpts) { return args, nil },
		&river.PeriodicJobOpts{RunOnStart: true})
}

// Deps are what the Enterprise jobs call, each built by the wiring for the rest of the instance.
type Deps struct {
	// Retention is the advanced-retention prune (ADR 0014).
	Retention *auditlogretention.UseCase
	// Detector is the abuse detector (ADR 0026).
	Detector *abusedetector.Detector
	// Audit is the Audit log, whose consumer stores an entry (ADR 0022).
	Audit *auditentries.Log
	// Erasures is the Erasures stream, whose consumer adds an Erasure and whose prune removes the
	// old ones (ADR 0045).
	Erasures *erasures.Module
	// Usage is the Usage the refresh materializes (ADR 0009).
	Usage *usages.Module
	// EventsRetention is the instance's Event retention window (0 keeps every row), past which the
	// Erasures stream is pruned (ADR 0045).
	EventsRetention time.Duration
}

// Extension is the Enterprise jobs for the job client: the advanced-retention prune (ADR 0014),
// which removes nothing without the retention license, the usage job (ADR 0009) and the abuse
// detector (ADR 0026), which do nothing without the staff license, the prune of the Erasures
// stream (ADR 0045), which does nothing without the export license, and the workers of the
// consumers (EventConsumers): the Audit log's, which stores nothing without the audit license,
// and the Erasures stream's, which stores nothing without the export license.
func Extension(deps Deps) jobextension.Extension {
	return jobextension.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, &retentionPruneWorker{retention: deps.Retention})
			river.AddWorker(w, &usageRefreshWorker{usage: deps.Usage})
			river.AddWorker(w, &abuseDetectionWorker{detector: deps.Detector})
			river.AddWorker(w, &exportPruneWorker{erasures: deps.Erasures, retention: deps.EventsRetention})
			river.AddWorker(w, NewEntryEventWorker(deps.Audit))
			river.AddWorker(w, NewErasureEventWorker(deps.Erasures))
		},
		PeriodicJobs: []*river.PeriodicJob{
			every(auditlogretention.Interval, RetentionPruneArgs{}),
			every(usages.Interval, UsageRefreshArgs{}),
			every(abusedetector.Interval, AbuseDetectionArgs{}),
			every(erasures.Interval, ExportPruneArgs{}),
		},
	}
}
