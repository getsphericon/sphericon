package erasure

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
)

// cancelInFlight stops work already under way for the subject and clears the internal
// queues of what refers to it: the Contact's recipients in unsent Broadcasts are
// removed (a Broadcast left with nothing pending settles), and the domain-event outbox
// rows and river job arguments naming the Contact, its runs or its recipients are
// deleted, as are the queued webhook deliveries whose payload names the Contact or
// its address, and the queued HTTP-step calls of its runs (a failure's response excerpt
// sits on the run, which Erasure deletes). A send a worker already handed to the provider is not recalled; one queued
// but not started finds its row gone and, at the send chokepoint, its Contact gone.
//
// It must run before the steps that delete the rows it reads ids from, and before the
// erasure publishes its own signal, which names no Contact.
func cancelInFlight(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	runIDs, err := s.AutomationRun().Query().Where(automationrun.ContactID(t.ContactID)).IDs(ctx)
	if err != nil {
		return err
	}
	unsent, err := s.BroadcastRecipient().Query().
		Where(broadcastrecipient.ContactID(t.ContactID), broadcasts.Pending()).
		All(ctx)
	if err != nil {
		return err
	}
	recipientIDs := lo.Map(unsent, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ID })
	affected := lo.Uniq(lo.Map(unsent, func(r *ent.BroadcastRecipient, _ int) int64 { return r.BroadcastID }))

	if err := purger.PurgeOutbox(ctx, s.WorkspaceID(), t.ContactID, t.Destinations); err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.EvaluateTrigger, "contact_id", []int64{t.ContactID}); err != nil {
		return err
	}
	endpointIDs, err := s.WebhookEndpoint().Query().IDs(ctx)
	if err != nil {
		return err
	}
	if err := purger.PurgeWebhookJobs(ctx, jobkind.DeliverWebhook, endpointIDs, t.ContactID, t.Destinations); err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.RunStep, "run_id", runIDs); err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.HTTPStep, "run_id", runIDs); err != nil {
		return err
	}
	if err := purger.PurgeJobs(ctx, jobkind.SendRecipient, "recipient_id", recipientIDs); err != nil {
		return err
	}

	if len(recipientIDs) == 0 {
		return nil
	}
	if _, err := s.BroadcastRecipient().Delete().Where(broadcastrecipient.IDIn(recipientIDs...)).Exec(ctx); err != nil {
		return err
	}
	for _, id := range affected {
		// The Broadcast's figures are read from its rows, so removing the recipients is all
		// they need; one left with nothing pending settles.
		if err := broadcasts.Finalize(ctx, s, id); err != nil {
			return err
		}
	}
	return nil
}
