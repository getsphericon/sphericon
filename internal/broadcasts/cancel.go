package broadcasts

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
)

// CancelPending cancels a Contact's pending Broadcast work (Erasure, ADR 0021): the
// recipients whose send has not ended are removed with their queued send jobs, and each
// Broadcast left with nothing pending settles. A recipient whose send ended stays as an
// anonymous record. It runs inside the caller's transaction (s and purger come from the
// same one); a send job that is already running is not purged, and finds its recipient
// gone when it starts.
func CancelPending(ctx context.Context, s *ent.Scoped, contactID int64, purger events.QueuePurger) error {
	unsent, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.ContactID(contactID), Pending()).
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
		if err := Finalize(ctx, s, id); err != nil {
			return err
		}
	}
	return nil
}
