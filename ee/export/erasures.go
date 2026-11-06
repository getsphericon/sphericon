package export

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
)

// EventConsumer is the Erasures stream's consumer of the domain events (ADR 0101): every
// `contact.erased` inserts one ErasureEventArgs job. The Edition registers it on the bus beside
// core's, so core never depends on the Enterprise Edition.
func EventConsumer() events.Consumer {
	return events.Consumer{
		Wants: func(env events.Envelope, _ events.DomainEvent) bool { return env.Name == events.NameContactErased },
		Job:   func(env events.Envelope) river.JobArgs { return ErasureEventArgs{events.EventJob{Envelope: env}} },
	}
}

// ErasureEventArgs is the Erasures stream's job: one Erasure to add to the stream (ADR 0045).
type ErasureEventArgs struct {
	events.EventJob
}

func (ErasureEventArgs) Kind() string { return "event_export_erasure" }

// ErasureEventWorker is the raw-client entry of the tombstone stream like the audit consumer (an
// envelope carries only a Workspace id, ADR 0017). It writes one Erasures row per
// `contact.erased`, deduplicated by the envelope id so a retried job writes one row. Unlicensed,
// it consumes and discards: an instance without the `export` feature stores nothing (ADR 0045).
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
	if !w.lic.Has(licensekey.FeatureExport) {
		return nil
	}
	env := job.Args.Envelope
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	erased, ok := ev.(*events.ContactErased)
	if !ok {
		return nil
	}
	return record(ctx, w.client, env, erased)
}

func record(ctx context.Context, client *ent.Client, env events.Envelope, e *events.ContactErased) error {
	create := client.Scoped(env.WorkspaceID).Erasure().Create().
		SetErasureKey(env.ID).
		SetSubjectID(e.SubjectID).
		SetVisitorIds(append([]string{}, e.VisitorIDs...)).
		SetErasedAt(env.OccurredAt)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	if e.EmailHash != "" {
		create.SetEmailSha256(e.EmailHash)
	}
	if e.PhoneHash != "" {
		create.SetPhoneSha256(e.PhoneHash)
	}
	if err := create.
		OnConflictColumns(erasure.FieldWorkspaceID, erasure.FieldErasureKey).
		Ignore().
		Exec(ctx); err != nil {
		return fmt.Errorf("record erasure %q: %w", env.ID, err)
	}
	return nil
}

// Interval is how often the prune job runs; the window is in days, so hourly is ample.
const Interval = time.Hour

// PruneArgs is the periodic prune job of the Erasures stream.
type PruneArgs struct{}

func (PruneArgs) Kind() string { return "ee_export_prune" }

// Worker runs Prune on the periodic tick, over the instance's Event retention window.
type Worker struct {
	river.WorkerDefaults[PruneArgs]

	client    *ent.Client
	lic       *licensekey.License
	retention time.Duration
}

// NewWorker builds the prune worker. It is a job entry point over every Workspace's
// Erasures rows, so it holds the raw client like ee/retention.
func NewWorker(client *ent.Client, lic *licensekey.License, retention time.Duration) *Worker {
	return &Worker{client: client, lic: lic, retention: retention}
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	_, err := Prune(ctx, w.client, w.lic, w.retention, time.Now())
	return err
}

// Periodic is the river schedule of the prune job.
func Periodic() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(Interval),
		func() (river.JobArgs, *river.InsertOpts) { return PruneArgs{}, nil },
		&river.PeriodicJobOpts{RunOnStart: true},
	)
}

// Jobs is the river extension of the Erasures stream: the prune job over the instance's
// Event retention window (ADR 0019, ADR 0045).
func Jobs(client *ent.Client, lic *licensekey.License, retention time.Duration) jobextension.Extension {
	return jobextension.Extension{
		Workers: func(w *river.Workers) {
			river.AddWorker(w, NewWorker(client, lic, retention))
			river.AddWorker(w, NewErasureEventWorker(client, lic))
		},
		PeriodicJobs: []*river.PeriodicJob{Periodic()},
	}
}

// Prune deletes the Erasures rows of every Workspace that are older than retention before now
// and returns how many it removed. Rows live for the Event retention window: past it the Events
// they announce are gone from a fresh read anyway, and a consumer that was away longer re-seeds
// from an empty cursor. A retention of zero or less keeps every row for good. Without the
// `export` license it removes nothing.
func Prune(ctx context.Context, client *ent.Client, lic *licensekey.License, retention time.Duration, now time.Time) (int, error) {
	if retention <= 0 || !lic.Has(licensekey.FeatureExport) {
		return 0, nil
	}
	n, err := client.Erasure.Delete().Where(erasure.ErasedAtLT(now.Add(-retention))).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune erasures: %w", err)
	}
	return n, nil
}
