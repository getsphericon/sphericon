package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
)

// RunContactImportWorker runs a CSV import of Contacts (ADR 0138): the module reads its file and
// writes its rows in batches.
type RunContactImportWorker struct {
	river.WorkerDefaults[contactimports.RunArgs]

	ent     *ent.Client
	imports *contactimports.Module
}

// NewRunContactImportWorker builds the worker that runs an import through imports.
func NewRunContactImportWorker(client *ent.Client, imports *contactimports.Module) *RunContactImportWorker {
	return &RunContactImportWorker{ent: client, imports: imports}
}

// Timeout gives a run the time a large file takes: each row is a transaction of its own, and a
// file of the Instance's largest size holds hundreds of thousands of rows.
func (w *RunContactImportWorker) Timeout(*river.Job[contactimports.RunArgs]) time.Duration {
	return 6 * time.Hour
}

// Work reads the import by id to learn its Workspace, before any scope exists, and runs it in that
// Workspace's scope (ADR 0017). An import that is gone (its Workspace erased) has nothing to run.
func (w *RunContactImportWorker) Work(ctx context.Context, job *river.Job[contactimports.RunArgs]) error {
	imp, err := w.ent.ContactImport.Get(ctx, job.Args.ImportID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return w.imports.Work(ctx, w.ent.Scoped(imp.WorkspaceID), imp.ID)
}
