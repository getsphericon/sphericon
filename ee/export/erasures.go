package export

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// ConsumerName names the bus consumer group of the Export streams' subscriber.
const ConsumerName = events.GroupExport

// Consumer is the Export streams' bus subscriber, the raw-client entry of the tombstone
// stream like the audit subscriber (an envelope carries only a Workspace id, ADR 0017). It
// writes one Erasures row per `contact.erased` message, deduplicated by the envelope id so
// at-least-once redelivery writes one row, and ignores every other event. Unlicensed, it
// consumes and discards: an instance without the `export` feature stores nothing (ADR 0045).
func Consumer(client *ent.Client, lic *licensekey.License) events.Consumer {
	return events.Consumer{
		Name: ConsumerName,
		Handle: func(ctx context.Context, env events.Envelope) error {
			if env.Name != events.NameContactErased || !lic.Has(licensekey.FeatureExport) {
				return nil
			}
			ev, err := events.Decode(env)
			if err != nil {
				return err
			}
			erased, ok := ev.(*events.ContactErased)
			if !ok {
				return nil
			}
			return record(ctx, client, env, erased)
		},
	}
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
func Jobs(client *ent.Client, lic *licensekey.License, retention time.Duration) jobs.Extension {
	return jobs.Extension{
		Workers:      func(w *river.Workers) { river.AddWorker(w, NewWorker(client, lic, retention)) },
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
