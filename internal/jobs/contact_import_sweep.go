package jobs

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/kernel/jobextension"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// SweepContactImportsArgs is the hourly tick over the CSV imports of every Workspace (ADR 0138): it
// deletes what they keep past its time (an upload that never ran after a day, a report seven days
// after its run ended) and queues a check of each running import's job.
type SweepContactImportsArgs struct{}

func (SweepContactImportsArgs) Kind() string { return "contact_imports_sweep" }

// SweepContactImportsWorker runs the sweep of the imports of every Workspace. It is Instance-wide,
// so it hands the module the raw ent client.
type SweepContactImportsWorker struct {
	river.WorkerDefaults[SweepContactImportsArgs]

	imports *contactimports.Module
	ent     *ent.Client
}

// NewSweepContactImportsWorker builds the sweep of the imports of imports over client.
func NewSweepContactImportsWorker(imports *contactimports.Module, client *ent.Client) *SweepContactImportsWorker {
	return &SweepContactImportsWorker{imports: imports, ent: client}
}

func (w *SweepContactImportsWorker) Work(ctx context.Context, _ *river.Job[SweepContactImportsArgs]) error {
	swept, err := w.imports.Sweep(ctx, w.ent)
	if err != nil {
		return err
	}
	if swept.Uploads > 0 || swept.Reports > 0 {
		slog.InfoContext(ctx, "expired contact imports pruned", "uploads", swept.Uploads, "reports", swept.Reports)
	}
	return jobextension.QueueEach(ctx, swept.Running, func(r contactimports.Running) river.JobArgs {
		return RecoverContactImportArgs{WorkspaceID: r.WorkspaceID, ImportID: r.ID}
	})
}

// RecoverContactImportArgs is the check of one running import's job: a run whose job is gone (its
// process killed on the job's last try, which river gives up on without its worker) is queued
// again (contactimports.Module.Recover).
type RecoverContactImportArgs struct {
	jobkind.Workspace

	ImportID int64 `json:"import_id"`
}

func (RecoverContactImportArgs) Kind() string { return "contact_import_recover" }

// InsertOpts queues one check of an import at a time (jobkind.QueuedOnce).
func (RecoverContactImportArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{UniqueOpts: jobkind.QueuedOnce()}
}

// RecoverContactImportWorker recovers one running import whose job is gone
// (contactimports.Module.Recover).
type RecoverContactImportWorker struct {
	river.WorkerDefaults[RecoverContactImportArgs]

	imports *contactimports.Module
	ent     *ent.Client
}

// NewRecoverContactImportWorker builds the check of an import's job over client.
func NewRecoverContactImportWorker(imports *contactimports.Module, client *ent.Client) *RecoverContactImportWorker {
	return &RecoverContactImportWorker{imports: imports, ent: client}
}

func (w *RecoverContactImportWorker) Work(ctx context.Context, job *river.Job[RecoverContactImportArgs]) error {
	// The Workspace comes from the job's args, which the sweep read from the import (ADR 0017).
	return w.imports.Recover(ctx, w.ent.Scoped(job.Args.WorkspaceID), job.Args.ImportID)
}
