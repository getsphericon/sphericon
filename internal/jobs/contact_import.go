package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
)

// contactImportTry is how long a try of an import's job writes batches before it yields to the
// next, which resumes from its checkpoint: a large file takes hours, and a try must end well
// within its timeout, which must stay below river's rescue of a job it takes for stuck (an hour).
const contactImportTry = 20 * time.Minute

// RunContactImportWorker runs a CSV import of Contacts (ADR 0138): the module reads its file and
// writes its rows in batches, a try at a time.
type RunContactImportWorker struct {
	river.WorkerDefaults[contactimports.RunArgs]

	ent     *ent.Client
	imports *contactimports.Module
	now     func() time.Time
}

// NewRunContactImportWorker builds the worker that runs an import through imports, its tries
// timed by now.
func NewRunContactImportWorker(client *ent.Client, imports *contactimports.Module, now func() time.Time) *RunContactImportWorker {
	return &RunContactImportWorker{ent: client, imports: imports, now: now}
}

// Timeout leaves a try the batch it is writing when it yields.
func (w *RunContactImportWorker) Timeout(*river.Job[contactimports.RunArgs]) time.Duration {
	return contactImportTry + 10*time.Minute
}

// Work reads the import by id to learn its Workspace, before any scope exists, and runs a try of
// it in that Workspace's scope (ADR 0017). An import that is gone (its Workspace erased) has
// nothing to run. A try that yields, or that a shutdown interrupts, is snoozed rather than retried,
// so that it spends none of the job's attempts; the last attempt's failure fails the import.
func (w *RunContactImportWorker) Work(ctx context.Context, job *river.Job[contactimports.RunArgs]) error {
	imp, err := w.ent.ContactImport.Get(ctx, job.Args.ImportID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	out, err := w.imports.Work(ctx, w.ent.Scoped(imp.WorkspaceID), contactimports.WorkInput{
		ImportID:    imp.ID,
		LastAttempt: job.Attempt >= job.MaxAttempts,
		Until:       w.now().Add(contactImportTry),
	})
	switch {
	case err != nil && errors.Is(ctx.Err(), context.Canceled), err == nil && out == contactimports.Paused:
		return river.JobSnooze(0)
	}
	return err
}
