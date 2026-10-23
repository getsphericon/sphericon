package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

// workspaceErasureInterval is how often the erasure job looks for Workspaces whose
// deletion window has passed; the window is 30 days, so hourly is ample.
const workspaceErasureInterval = time.Hour

// EraseWorkspacesArgs is the periodic tick that erases Workspaces whose deletion
// window has passed (ADR 0043).
type EraseWorkspacesArgs struct{}

func (EraseWorkspacesArgs) Kind() string { return "workspace_erasure" }

// EraseWorkspacesWorker triggers the Workspace deletion module's erase with the clock.
type EraseWorkspacesWorker struct {
	river.WorkerDefaults[EraseWorkspacesArgs]

	deletion *workspacedeletion.Module
	now      func() time.Time
}

func (w *EraseWorkspacesWorker) Work(ctx context.Context, _ *river.Job[EraseWorkspacesArgs]) error {
	n, err := w.deletion.EraseDue(ctx, w.now())
	if n > 0 {
		slog.InfoContext(ctx, "workspaces erased", "count", n)
	}
	return err
}
