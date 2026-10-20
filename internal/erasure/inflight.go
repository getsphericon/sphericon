package erasure

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
)

// cancelInFlight stops work already under way for the subject and clears the internal
// queues of what refers to it: the Broadcast delivery module cancels the Contact's unsent
// recipients (a Broadcast left with nothing pending settles), the Automation module
// cancels the queued step calls and trigger evaluations (a failure's response excerpt
// sits on the run, which Erasure deletes), and the domain-event outbox rows and the queued
// webhook deliveries whose payload names the Contact or its address are deleted. A send a worker already handed to the provider is not recalled; one
// queued but not started finds its row gone and, at the send chokepoint, its Contact gone.
//
// It must run before the steps that delete the rows it reads ids from, and before the
// erasure publishes its own signal, which names no Contact.
func cancelInFlight(ctx context.Context, s *ent.Scoped, t *Target, purger events.PurgingPublisher) error {
	if err := broadcasts.CancelPending(ctx, s, t.ContactID, purger); err != nil {
		return err
	}
	if err := purger.PurgeOutbox(ctx, s.WorkspaceID(), t.ContactID, t.Destinations); err != nil {
		return err
	}
	if err := automations.CancelPending(ctx, s, t.ContactID, purger); err != nil {
		return err
	}
	endpointIDs, err := s.WebhookEndpoint().Query().IDs(ctx)
	if err != nil {
		return err
	}
	if err := purger.PurgeWebhookJobs(ctx, endpointIDs, t.ContactID, t.Destinations); err != nil {
		return err
	}
	return nil
}
