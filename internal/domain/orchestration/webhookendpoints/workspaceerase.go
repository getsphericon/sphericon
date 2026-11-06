package webhookendpoints

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseWebhookEndpointsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.WebhookEndpoint().Delete().Where(webhookendpoint.IDIn(ids...)).Exec(ctx)
	return err
}
