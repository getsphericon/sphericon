// Package broadcastsunchecked holds the cancellation of a Contact's pending Broadcast work. It is a
// step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use
// case internal/usecase/contacterasure runs each owner's step in its order, in the one transaction
// of the Erasure. It also holds the steps of message retention's prune of a finished Broadcast (ADR
// 0143), which internal/usecase/messageretention runs. Only a use case may import this one (ADR
// 0110), and each that does builds its Module. The broadcasts module does not import it.
package broadcastsunchecked

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// Module is the broadcasts module's unchecked operations (docs/architecture/modules.md): the
// cancellation of a Contact's pending work and the steps of a finished Broadcast's prune.
type Module struct {
	catalogue *broadcasts.Module
	bus       *events.Bus
	now       func() time.Time
}

// New builds the unchecked operations over the Broadcasts they settle and read the figures of, the
// bus whose Erasure transaction purges a cancelled recipient's send job, and the clock a prune is
// marked by.
func New(catalogue *broadcasts.Module, bus *events.Bus, now func() time.Time) *Module {
	return &Module{catalogue: catalogue, bus: bus, now: now}
}

// CancelPending cancels a Contact's pending Broadcast work (Erasure, ADR 0021): the
// recipients whose send has not ended are removed with their queued send jobs, and each
// Broadcast left with nothing pending settles. A recipient whose send ended stays, and becomes an
// anonymous record when the Contact row goes: its foreign key sets the reference null. It runs
// inside the Erasure's transaction, which s is bound to (events.Bus.WithinScopedPurgeTx), and
// purges the send jobs in it; a send job that is already running is not purged, and finds its
// recipient gone when it starts.
func (m *Module) CancelPending(ctx context.Context, s *ent.Scoped, contactID int64) error {
	unsent, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.ContactID(contactID), broadcasts.Pending()).
		All(ctx)
	if err != nil {
		return err
	}
	if len(unsent) == 0 {
		return nil
	}
	ids := lo.Map(unsent, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ID })
	affected := lo.Uniq(lo.Map(unsent, func(r *ent.BroadcastRecipient, _ int) int64 { return r.BroadcastID }))

	if err := m.bus.PurgeJobsIn(ctx, s, jobkind.SendRecipient, ids); err != nil {
		return err
	}
	if _, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	for _, id := range affected {
		// The Broadcast's figures are read from its rows, so removing the recipients is all
		// they need; one left with nothing pending settles.
		if err := m.catalogue.Finalize(ctx, s, id); err != nil {
			return err
		}
	}
	return nil
}
