package app

import (
	"context"
	"errors"

	"github.com/samber/do/v2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// EventJournalRequest selects exactly one Workspace by public id or explicitly the Instance.
type EventJournalRequest struct {
	Workspace string
	Instance  bool
	Query     events.JournalQuery
}

// EventJournal inspects one scope's retained publication metadata through the bus.
func (a *App) EventJournal(ctx context.Context, in EventJournalRequest) (events.JournalPage, error) {
	if (in.Workspace != "") == in.Instance {
		return events.JournalPage{}, errors.New("events: select one Workspace or the Instance")
	}
	bus, err := do.Invoke[*events.Bus](a.injector)
	if err != nil {
		return events.JournalPage{}, err
	}
	if in.Instance {
		return bus.InstanceJournal(ctx, in.Query)
	}
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return events.JournalPage{}, err
	}
	id, err := workspaces.IDByPublicID(ctx, client, in.Workspace)
	if err != nil {
		return events.JournalPage{}, err
	}
	return bus.WorkspaceJournal(ctx, client.Scoped(id), in.Query)
}
