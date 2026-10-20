package jobs_test

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// planAudience plans a Broadcast through the module and returns its recipient ids, as the
// send worker does before it enqueues the recipient jobs.
func planAudience(ctx context.Context, client *ent.Client, mod *outbound.Module, broadcastID int64) ([]int64, error) {
	b, err := client.Broadcast.Get(ctx, broadcastID)
	if err != nil {
		return nil, err
	}
	return broadcasts.NewDelivery(mod).Plan(ctx, client.Scoped(b.WorkspaceID), broadcastID)
}

// deliverRecipient sends one recipient through the module, as the recipient worker does.
func deliverRecipient(ctx context.Context, client *ent.Client, mod *outbound.Module, recipientID int64) error {
	rec, err := client.BroadcastRecipient.Get(ctx, recipientID)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return broadcasts.NewDelivery(mod).SendRecipient(ctx, client.Scoped(rec.WorkspaceID), recipientID)
}
