package jobs_test

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// deliveryOf is the Broadcast delivery over mod, its sends scheduled as recipient jobs, as the
// wiring builds it.
func deliveryOf(mod *outboundmessages.Module) *broadcasts.Delivery {
	return broadcasts.NewDelivery(mod, jobs.RecipientJobs{})
}

// planAudience plans a Broadcast through the module and returns its recipient ids, as the
// send worker does before it enqueues the recipient jobs.
func planAudience(ctx context.Context, client *ent.Client, mod *outboundmessages.Module, broadcastID int64) ([]int64, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return nil, err
	}
	return deliveryOf(mod).Plan(ctx, client.Scoped(b.WorkspaceID), broadcastID)
}

// deliverRecipient sends one recipient through the module, as the recipient worker does.
func deliverRecipient(ctx context.Context, client *ent.Client, mod *outboundmessages.Module, recipientID int64) error {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return deliveryOf(mod).SendRecipient(ctx, client.Scoped(rec.WorkspaceID), recipientID)
}
