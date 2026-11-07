package ee

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/domain/insights/auditentries"
	"github.com/getsphericon/sphericon/ee/domain/insights/erasures"
	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usecase/abusedetection"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
)

// The Edition's jobs: the workers and periodic schedules of its modules, and the jobs of its
// consumers of the domain events. The modules and use cases do the work; how it is scheduled is
// the Edition's, which hands it to the job client as a jobextension.Extension (ADR 0102).

// EventConsumers are the Edition's consumers of the domain events (ADR 0101): the Audit log's and
// the Erasures stream's, for the composition root to register on the bus beside core's. They want
// what they want whatever the license: each job checks the license itself (Jobs, ExportJobs), so
// a key added later needs no rewiring and an unlicensed instance stores nothing.
func EventConsumers() []events.Consumer {
	return []events.Consumer{
		{
			Wants: func(env events.Envelope, _ events.DomainEvent) bool { return env.Name == events.NameAuditEntry },
			Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
				return EntryEventArgs{events.EventJob{Envelope: env}}
			},
		},
		{
			Wants: func(env events.Envelope, _ events.DomainEvent) bool { return env.Name == events.NameContactErased },
			Job: func(env events.Envelope, _ events.DomainEvent) river.JobArgs {
				return ErasureEventArgs{events.EventJob{Envelope: env}}
			},
		},
	}
}

// EntryEventArgs is the Audit log's job: one Audit entry to store (ADR 0022).
type EntryEventArgs struct {
	events.EventJob
}

func (EntryEventArgs) Kind() string { return "event_audit" }

// EntryEventWorker stores the Audit entry of its event (auditentries.Record).
type EntryEventWorker struct {
	river.WorkerDefaults[EntryEventArgs]

	client *ent.Client
	lic    *licensekey.License
}

// NewEntryEventWorker builds the Audit log's consumer over the raw client.
func NewEntryEventWorker(client *ent.Client, lic *licensekey.License) *EntryEventWorker {
	return &EntryEventWorker{client: client, lic: lic}
}

func (w *EntryEventWorker) Work(ctx context.Context, job *river.Job[EntryEventArgs]) error {
	return auditentries.Record(ctx, w.client, w.lic, job.Args.Envelope)
}

// ErasureEventArgs is the Erasures stream's job: one Erasure to add to the stream (ADR 0045).
type ErasureEventArgs struct {
	events.EventJob
}

func (ErasureEventArgs) Kind() string { return "event_export_erasure" }

// ErasureEventWorker adds the Erasure of its event to the stream (erasures.Record).
type ErasureEventWorker struct {
	river.WorkerDefaults[ErasureEventArgs]

	client *ent.Client
	lic    *licensekey.License
}

// NewErasureEventWorker builds the Erasures stream's consumer over the raw client.
func NewErasureEventWorker(client *ent.Client, lic *licensekey.License) *ErasureEventWorker {
	return &ErasureEventWorker{client: client, lic: lic}
}

func (w *ErasureEventWorker) Work(ctx context.Context, job *river.Job[ErasureEventArgs]) error {
	return erasures.Record(ctx, w.client, w.lic, job.Args.Envelope)
}

// RetentionPruneArgs is the advanced-retention prune (ADR 0014), every auditlogretention.Interval.
type RetentionPruneArgs struct{}

func (RetentionPruneArgs) Kind() string { return "ee_retention_prune" }

type retentionPruneWorker struct {
	river.WorkerDefaults[RetentionPruneArgs]

	retention *auditlogretention.UseCase
}

func (w *retentionPruneWorker) Work(ctx context.Context, _ *river.Job[RetentionPruneArgs]) error {
	_, err := w.retention.Prune(ctx)
	return err
}

// UsageRefreshArgs closes the elapsed Usage periods and refreshes the open one (ADR 0009), every
// usages.Interval.
type UsageRefreshArgs struct{}

func (UsageRefreshArgs) Kind() string { return "ee_usage_refresh" }

type usageRefreshWorker struct {
	river.WorkerDefaults[UsageRefreshArgs]

	client *ent.Client
	lic    *licensekey.License
	now    func() time.Time
}

func newUsageRefreshWorker(client *ent.Client, lic *licensekey.License, now func() time.Time) *usageRefreshWorker {
	return &usageRefreshWorker{client: client, lic: lic, now: now}
}

func (w *usageRefreshWorker) Work(ctx context.Context, _ *river.Job[UsageRefreshArgs]) error {
	return usages.Refresh(ctx, w.client, w.lic, w.now())
}

// AbuseDetectionArgs is a pass of the abuse detector (ADR 0026), every abusedetection.Interval.
type AbuseDetectionArgs struct{}

func (AbuseDetectionArgs) Kind() string { return "ee_abuse_detector" }

type abuseDetectionWorker struct {
	river.WorkerDefaults[AbuseDetectionArgs]

	detector *abusedetection.UseCase
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

	client    *ent.Client
	lic       *licensekey.License
	retention time.Duration
	now       func() time.Time
}

func newExportPruneWorker(client *ent.Client, lic *licensekey.License, retention time.Duration, now func() time.Time) *exportPruneWorker {
	return &exportPruneWorker{client: client, lic: lic, retention: retention, now: now}
}

func (w *exportPruneWorker) Work(ctx context.Context, _ *river.Job[ExportPruneArgs]) error {
	_, err := erasures.Prune(ctx, w.client, w.lic, w.retention, w.now())
	return err
}

// every is the periodic schedule of args every interval, which also runs at start.
func every(interval time.Duration, args river.JobArgs) *river.PeriodicJob {
	return river.NewPeriodicJob(river.PeriodicInterval(interval),
		func() (river.JobArgs, *river.InsertOpts) { return args, nil },
		&river.PeriodicJobOpts{RunOnStart: true})
}

// Jobs is the Edition's river extension: the advanced-retention prune job (ADR 0014),
// which removes nothing without the retention license, the usage job (ADR 0009) and the
// abuse detector (ADR 0026), which do nothing without the staff license, and the Audit log's
// consumer (EventConsumers), which stores nothing without the audit license.
func (e *Edition) Jobs() jobextension.Extension {
	detector := e.Abuse()
	return jobextension.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, &retentionPruneWorker{retention: auditlogretention.New(e.client, e.lic, e.now)})
			river.AddWorker(w, newUsageRefreshWorker(e.client, e.lic, e.now))
			river.AddWorker(w, &abuseDetectionWorker{detector: detector})
			river.AddWorker(w, NewEntryEventWorker(e.client, e.lic))
		},
		PeriodicJobs: []*river.PeriodicJob{
			every(auditlogretention.Interval, RetentionPruneArgs{}),
			every(usages.Interval, UsageRefreshArgs{}),
			every(abusedetection.Interval, AbuseDetectionArgs{}),
		},
	}
}

// ExportJobs is the river extension of the Export streams (ADR 0045): the prune job of the
// Erasures stream over the instance's Event retention window (0 keeps every row), and the
// stream's consumer of the domain events (EventConsumers). It is separate from Jobs because the
// prune needs that window, a core setting; neither does anything without the `export` license.
func (e *Edition) ExportJobs(eventsRetention time.Duration) jobextension.Extension {
	return jobextension.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, newExportPruneWorker(e.client, e.lic, eventsRetention, e.now))
			river.AddWorker(w, NewErasureEventWorker(e.client, e.lic))
		},
		PeriodicJobs: []*river.PeriodicJob{every(erasures.Interval, ExportPruneArgs{})},
	}
}
