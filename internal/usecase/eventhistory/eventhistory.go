// Package eventhistory is the use case of the operator's Event history (ADR 0116): reading the
// journal's publication metadata of a Workspace or of the Instance, and replaying one report
// consumer for a Workspace (`sphericon events list` and `replay`). The journal is the bus's
// (internal/infra/events), which imports no domain module, and the Workspace an operator names by
// its id (ADR 0043) is identity's, so the use case resolves the id through workspaces and hands
// the bus the Workspace. The replay checks again, under the Workspace's lock in its own
// transaction, that the Workspace is neither deleted nor erased.
package eventhistory

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase reads and replays the Event history.
type UseCase struct {
	workspaces *workspaces.Module
	bus        *events.Bus
}

// New builds the use case over the Workspaces that resolve an operator's id and the bus that
// keeps the journal.
func New(ws *workspaces.Module, bus *events.Bus) *UseCase {
	return &UseCase{workspaces: ws, bus: bus}
}

// Of names whose history List reads: the Instance's own facts, or one Workspace's by its id
// (ADR 0043). Never every Workspace's.
type Of struct {
	Instance  bool
	Workspace string
}

// List reads the publication metadata of the Instance, or of the Workspace the id names, whatever
// its deletion state. An id that names no Workspace, an empty one included, is
// workspaces.ErrNotFound: it is never read as the Instance.
func (u *UseCase) List(ctx context.Context, of Of, q events.JournalQuery) (events.JournalPage, error) {
	if of.Instance {
		return u.bus.InstanceJournal(ctx, q)
	}
	id, err := u.workspaces.IDByPublicID(ctx, of.Workspace)
	if err != nil {
		return events.JournalPage{}, err
	}
	return u.bus.WorkspaceJournal(ctx, id, q)
}

// Replay queues one report consumer's jobs again for the events the Workspace with the id
// workspace recorded in the request's interval (events.Bus.Replay). A Workspace that is unknown,
// deleted or erased is events.ErrReplayWorkspaceUnavailable.
func (u *UseCase) Replay(ctx context.Context, workspace string, req events.ReplayRequest) (events.ReplayResult, error) {
	id, err := u.workspaces.IDByPublicID(ctx, workspace)
	if errors.Is(err, workspaces.ErrNotFound) {
		err = events.ErrReplayWorkspaceUnavailable
	}
	if err != nil {
		return events.ReplayResult{}, fmt.Errorf("workspace %q: %w", workspace, err)
	}
	result, err := u.bus.Replay(ctx, id, req)
	if errors.Is(err, events.ErrReplayWorkspaceUnavailable) {
		err = fmt.Errorf("workspace %q: %w", workspace, err)
	}
	return result, err
}
