// Package retention is the Enterprise advanced-retention control (ADR 0014): a
// Workspace sets a window in days (workspaces.retention_days), and one periodic river
// job removes the Enterprise records older than it. Today the only such record is the
// Audit log (ADR 0022), whose rows are otherwise append-only: this job is the single
// remover, and there is no audit-specific engine, only this control. Unlicensed, the
// job prunes nothing.
//
// Governed by ee/LICENSE, not the AGPL.
package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// Interval is how often the prune job runs; the window is in days, so hourly is ample.
const Interval = time.Hour

// PruneArgs is the periodic prune job.
type PruneArgs struct{}

func (PruneArgs) Kind() string { return "ee_retention_prune" }

// Worker runs Prune on the periodic tick.
type Worker struct {
	river.WorkerDefaults[PruneArgs]
	client *ent.Client
	lic    *licensekey.License
}

// NewWorker builds the prune worker. It is the raw-client entry of the control: like a
// job entry point it lists Workspaces, then scopes to each one.
func NewWorker(client *ent.Client, lic *licensekey.License) *Worker {
	return &Worker{client: client, lic: lic}
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[PruneArgs]) error {
	_, err := Prune(ctx, w.client, w.lic, time.Now())
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

// Prune deletes, in every Workspace that set a retention window, the Audit entries that
// occurred before now minus that window, and returns how many it removed. A Workspace
// with no window keeps everything. Without the retention license it removes nothing.
func Prune(ctx context.Context, client *ent.Client, lic *licensekey.License, now time.Time) (int, error) {
	if !lic.Has(licensekey.FeatureRetention) {
		return 0, nil
	}
	workspaces, err := client.Workspace.Query().
		Where(workspace.RetentionDaysNotNil(), workspace.DiscardedAtIsNil()).
		Select(workspace.FieldID, workspace.FieldRetentionDays).
		All(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces with a retention window: %w", err)
	}
	var total int
	for _, ws := range workspaces {
		cutoff := now.AddDate(0, 0, -*ws.RetentionDays)
		n, err := client.Scoped(ws.ID).AuditEntry().Delete().
			Where(auditentry.OccurredAtLT(cutoff)).
			Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("prune audit entries of workspace %d: %w", ws.ID, err)
		}
		total += n
	}
	return total, nil
}
