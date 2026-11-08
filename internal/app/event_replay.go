package app

import (
	"context"

	"github.com/samber/do/v2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// ReplayEvents is the operator's bounded reapplication of one anonymous report consumer.
// The Workspace id is resolved here; the bus owns selection, validation and atomic enqueue.
func (a *App) ReplayEvents(ctx context.Context, publicID string, in events.ReplayRequest) (events.ReplayResult, error) {
	client, err := do.Invoke[*ent.Client](a.injector)
	if err != nil {
		return events.ReplayResult{}, err
	}
	id, err := workspaces.IDByPublicID(ctx, client, publicID)
	if err != nil {
		return events.ReplayResult{}, err
	}
	bus, err := do.Invoke[*events.Bus](a.injector)
	if err != nil {
		return events.ReplayResult{}, err
	}
	return bus.Replay(ctx, client.Scoped(id), in)
}
