package jobs

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
)

// SweepContactImportsArgs is the hourly tick over the CSV imports of every Workspace (ADR 0138): it
// deletes what they keep past its time (an upload that never ran after a day, a report seven days
// after its run ended), and queues again a run whose job is gone.
type SweepContactImportsArgs struct{}

func (SweepContactImportsArgs) Kind() string { return "contact_imports_sweep" }

// SweepContactImportsWorker runs the sweep of the imports of every Workspace. It is Instance-wide,
// so it hands the module the raw ent client; it reads river's queue for the jobs of the runs.
type SweepContactImportsWorker struct {
	river.WorkerDefaults[SweepContactImportsArgs]

	imports *contactimports.Module
	ent     *ent.Client
}

// NewSweepContactImportsWorker builds the sweep of the imports of imports over client.
func NewSweepContactImportsWorker(imports *contactimports.Module, client *ent.Client) *SweepContactImportsWorker {
	return &SweepContactImportsWorker{imports: imports, ent: client}
}

// liveRunStates are the states of a run's job that will still work it.
var liveRunStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRetryable,
	rivertype.JobStateRunning, rivertype.JobStateScheduled,
}

func (w *SweepContactImportsWorker) Work(ctx context.Context, _ *river.Job[SweepContactImportsArgs]) error {
	expired, err := w.imports.Expire(ctx, w.ent)
	if err != nil {
		return err
	}
	if expired.Uploads > 0 || expired.Reports > 0 {
		slog.InfoContext(ctx, "expired contact imports pruned", "uploads", expired.Uploads, "reports", expired.Reports)
	}
	running, err := w.imports.RunningIn(ctx, w.ent)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*sql.Tx](ctx)
	for _, imp := range running {
		live, err := rc.JobList(ctx, river.NewJobListParams().
			Kinds(contactimports.RunArgs{}.Kind()).
			States(liveRunStates...).
			Where("args->>'import_id' = @import_id", river.NamedArgs{"import_id": strconv.FormatInt(imp.ID, 10)}).
			First(1))
		if err != nil {
			return err
		}
		if len(live.Jobs) > 0 {
			continue
		}
		slog.WarnContext(ctx, "contact import run without a job recovered", "import_id", imp.ID)
		// The Workspace comes from the import's row, read before any scope exists (ADR 0017).
		if err := w.imports.Recover(ctx, w.ent.Scoped(imp.WorkspaceID), imp.ID); err != nil {
			return err
		}
	}
	return nil
}
