package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// A job of a Workspace names it at jobkind.WorkspaceArg, where the erase of the Workspace finds and
// deletes it (events.WorkspacePurger). Two guards keep that true at the ends a purge cannot reach:
// a job is never queued without its Workspace, and a job of an erased Workspace never runs.

// ErrNoWorkspace refuses to queue a job of a Workspace (jobkind.OfWorkspace) whose Workspace is 0:
// the erase of its Workspace would never find it.
var ErrNoWorkspace = errors.New("jobs: a job of a Workspace names no Workspace")

// errWorkspaceErased cancels a job whose Workspace was erased after it was queued.
var errWorkspaceErased = errors.New("jobs: the job's Workspace is erased")

// workspaceNamed is the insert middleware that refuses a job of a Workspace without one, on
// every insert of the application (BaseConfig): a producer that forgets the Workspace fails at
// once instead of leaving a job no erase removes.
type workspaceNamed struct {
	river.MiddlewareDefaults
}

var _ rivertype.JobInsertMiddleware = (*workspaceNamed)(nil)

func (*workspaceNamed) InsertMany(ctx context.Context, params []*rivertype.JobInsertParams, doInner func(context.Context) ([]*rivertype.JobInsertResult, error)) ([]*rivertype.JobInsertResult, error) {
	for _, p := range params {
		if owned, ok := p.Args.(jobkind.OfWorkspace); ok && owned.JobWorkspace() == 0 {
			return nil, fmt.Errorf("%w: %s", ErrNoWorkspace, p.Kind)
		}
	}
	return doInner(ctx)
}

// erasedWorkspace is the worker middleware that cancels a job of an erased Workspace before its
// worker runs (WorkerConfig). The erase deletes every job that waits; this catches one that a
// worker fetched before the purge and that runs again after it (a retry), and an event consumer
// that writes into the envelope's Workspace without loading a row (a suppression, an Order, an
// Audit entry), which would otherwise write behind the tombstone.
type erasedWorkspace struct {
	river.MiddlewareDefaults

	client *ent.Client
}

var _ rivertype.WorkerMiddleware = (*erasedWorkspace)(nil)

func (m *erasedWorkspace) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	var named jobkind.Workspace
	if err := json.Unmarshal(job.EncodedArgs, &named); err != nil || named.WorkspaceID == 0 {
		return doInner(ctx) // a job of the Instance; args that do not decode fail in the worker
	}
	erased, err := workspaces.Erased(ctx, m.client, named.WorkspaceID)
	if err != nil {
		return err
	}
	if erased {
		return river.JobCancel(fmt.Errorf("%w: workspace %d", errWorkspaceErased, named.WorkspaceID))
	}
	return doInner(ctx)
}
