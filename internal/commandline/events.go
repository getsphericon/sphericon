package commandline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// EventHistory is what `sphericon events list` and `replay` read and replay through (ADR 0116):
// the journal is the bus's, which imports no domain module, and the Workspace an operator names
// by its id (ADR 0043) is resolved through workspaces, once, before the bus is asked.
type EventHistory struct {
	Workspaces *workspaces.Module
	Bus        *events.Bus
}

// JournalOf names whose journal List reads: the Instance's own facts, or one Workspace's by its
// id. Never every Workspace's.
type JournalOf struct {
	Instance  bool
	Workspace string
}

// List prints, as JSON, the publication metadata of the Instance or of the Workspace the id
// names, whatever its deletion state. An id that names no Workspace, an empty one included, is
// workspaces.ErrNotFound: it is never read as the Instance.
func (h EventHistory) List(ctx context.Context, w io.Writer, of JournalOf, q events.JournalQuery) error {
	page, err := h.journal(ctx, of, q)
	if err != nil {
		return err
	}
	return json.NewEncoder(w).Encode(page)
}

func (h EventHistory) journal(ctx context.Context, of JournalOf, q events.JournalQuery) (events.JournalPage, error) {
	if of.Instance {
		return h.Bus.InstanceJournal(ctx, q)
	}
	id, err := h.Workspaces.IDByPublicID(ctx, of.Workspace)
	if err != nil {
		return events.JournalPage{}, err
	}
	return h.Bus.WorkspaceJournal(ctx, id, q)
}

// Replay queues one report consumer's jobs again for the events the Workspace with the id
// workspace recorded in the request's interval (events.Bus.Replay), and prints what it matched
// and queued. A Workspace that is unknown, deleted or erased is
// events.ErrReplayWorkspaceUnavailable; the bus checks the last two again under the Workspace's
// lock in its own transaction.
func (h EventHistory) Replay(ctx context.Context, w io.Writer, workspace string, req events.ReplayRequest) error {
	id, err := h.Workspaces.IDByPublicID(ctx, workspace)
	if errors.Is(err, workspaces.ErrNotFound) {
		err = events.ErrReplayWorkspaceUnavailable
	}
	if err != nil {
		return fmt.Errorf("workspace %q: %w", workspace, err)
	}
	result, err := h.Bus.Replay(ctx, id, req)
	if errors.Is(err, events.ErrReplayWorkspaceUnavailable) {
		return fmt.Errorf("workspace %q: %w", workspace, err)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "matched %d; enqueued %d\n", result.Matched, result.Enqueued)
	return err
}
