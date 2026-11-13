package jobs

import (
	"context"
	stdsql "database/sql"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/usecase/messageretention"
)

// messagesRetentionSchedule is when the message retention tick runs: daily at an off-peak hour,
// 05:00 UTC, apart from the Event prune.
var messagesRetentionSchedule = mustParseCron("CRON_TZ=UTC 0 5 * * *")

// PruneMessagesArgs is the daily tick of message retention (ADR 0143).
type PruneMessagesArgs struct{}

func (PruneMessagesArgs) Kind() string { return "messages_retention" }

// PruneMessagesWorker fans the tick out into one PruneWorkspaceMessages job per Workspace with
// something past the window (ADR 0144): a failure stays in its Workspace, and a larger Instance
// means more jobs, not a longer one.
type PruneMessagesWorker struct {
	river.WorkerDefaults[PruneMessagesArgs]

	retention *messageretention.UseCase
}

// NewPruneMessagesWorker builds the tick over the use case.
func NewPruneMessagesWorker(retention *messageretention.UseCase) *PruneMessagesWorker {
	return &PruneMessagesWorker{retention: retention}
}

func (w *PruneMessagesWorker) Work(ctx context.Context, _ *river.Job[PruneMessagesArgs]) error {
	due, err := w.retention.Due(ctx)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, ws := range due {
		if _, err := rc.Insert(ctx, PruneWorkspaceMessagesArgs{WorkspaceID: ws}, nil); err != nil {
			return err
		}
	}
	return nil
}

// PruneWorkspaceMessagesArgs is message retention's work in one Workspace.
type PruneWorkspaceMessagesArgs struct {
	jobkind.Workspace
}

func (PruneWorkspaceMessagesArgs) Kind() string { return "messages_retention_workspace" }

// PruneWorkspaceMessagesWorker deletes one Workspace's messages past the window, keeping their
// figures, in batches that persist at once (ADR 0144). A run works at most a budget of batches,
// so it fits its timeout, and queues the next run while more is left: the progress is the work
// itself, and the next run starts from what is still there.
type PruneWorkspaceMessagesWorker struct {
	river.WorkerDefaults[PruneWorkspaceMessagesArgs]

	retention *messageretention.UseCase
}

// NewPruneWorkspaceMessagesWorker builds the worker over the use case.
func NewPruneWorkspaceMessagesWorker(retention *messageretention.UseCase) *PruneWorkspaceMessagesWorker {
	return &PruneWorkspaceMessagesWorker{retention: retention}
}

func (w *PruneWorkspaceMessagesWorker) Work(ctx context.Context, job *river.Job[PruneWorkspaceMessagesArgs]) error {
	pruned, err := w.retention.Prune(ctx, job.Args.WorkspaceID, messageretention.Budget)
	if err != nil {
		return err
	}
	if pruned.Broadcast+pruned.Automation+pruned.Transactional > 0 {
		slog.InfoContext(ctx, "expired outbound messages pruned", "workspace_id", job.Args.WorkspaceID,
			"broadcast", pruned.Broadcast, "automation", pruned.Automation, "transactional", pruned.Transactional)
	}
	if !pruned.More {
		return nil
	}
	_, err = river.ClientFromContext[*stdsql.Tx](ctx).Insert(ctx, job.Args, nil)
	return err
}
