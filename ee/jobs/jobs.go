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
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usecase/auditlogretention"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
)

// EventConsumers are the Edition's consumers of the domain events (ADR 0101): the Audit log's and
// the Erasures stream's, for the composition root to register on the bus beside core's. They want
// what they want whatever the license: each job checks the license itself, so a key added later
// needs no rewiring and an unlicensed instance stores nothing.
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

// Deps are what the Enterprise jobs call, each built by the wiring for the rest of the instance.
type Deps struct {
	// Client is the raw ent client the Enterprise modules work across Workspaces with.
	Client *ent.Client
	// License is the runtime license key; nil is the plain core, and every job does nothing.
	License *licensekey.License
	// Retention is the advanced-retention prune (ADR 0014).
	Retention *auditlogretention.UseCase
	// Detector is the abuse detector (ADR 0026).
	Detector *abusedetector.Detector
	// EventsRetention is the instance's Event retention window (0 keeps every row), past which the
	// Erasures stream is pruned (ADR 0045).
	EventsRetention time.Duration
	// Clock is the Instance clock.
	Clock func() time.Time
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
			river.AddWorker(w, newUsageRefreshWorker(deps.Client, deps.License, deps.Clock))
			river.AddWorker(w, &abuseDetectionWorker{detector: deps.Detector})
			river.AddWorker(w, newExportPruneWorker(deps.Client, deps.License, deps.EventsRetention, deps.Clock))
			river.AddWorker(w, NewEntryEventWorker(deps.Client, deps.License))
			river.AddWorker(w, NewErasureEventWorker(deps.Client, deps.License))
		},
		PeriodicJobs: []*river.PeriodicJob{
			every(auditlogretention.Interval, RetentionPruneArgs{}),
			every(usages.Interval, UsageRefreshArgs{}),
			every(abusedetector.Interval, AbuseDetectionArgs{}),
			every(erasures.Interval, ExportPruneArgs{}),
		},
	}
}
