package webhooks

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
)

// EraseWebhookEndpointsIn removes the scoped Workspace's Webhook endpoints with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseWebhookEndpointsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.WebhookEndpoint().Delete().Where(webhookendpoint.IDIn(ids...)).Exec(ctx)
	return err
}
