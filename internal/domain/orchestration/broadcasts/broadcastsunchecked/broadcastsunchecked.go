// Package broadcastsunchecked holds the cancellation of a Contact's pending Broadcast work. It is a
// step of a Contact's Erasure (ADR 0021), which harms a live Contact when it runs alone: the use
// case internal/usecase/contacterasure runs each owner's step in its order, in the one transaction
// of the Erasure; only a use case may import this one (ADR 0110). The broadcasts module does not
// import it.
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

// CancelPending cancels a Contact's pending Broadcast work (Erasure, ADR 0021): the
// recipients whose send has not ended are removed with their queued send jobs, and each
// Broadcast left with nothing pending settles. A recipient whose send ended stays, and becomes an
// anonymous record when the Contact row goes: its foreign key sets the reference null. It runs inside the caller's transaction (s and purger come from the
// same one); a send job that is already running is not purged, and finds its recipient
// gone when it starts.
func CancelPending(ctx context.Context, s *ent.Scoped, contactID int64, purger events.QueuePurger, at time.Time) error {
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

	if err := purger.PurgeJobs(ctx, jobkind.SendRecipient, ids); err != nil {
		return err
	}
	if _, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	for _, id := range affected {
		// The Broadcast's figures are read from its rows, so removing the recipients is all
		// they need; one left with nothing pending settles.
		if err := broadcasts.Finalize(ctx, s, id, at); err != nil {
			return err
		}
	}
	return nil
}
