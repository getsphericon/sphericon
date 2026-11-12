package webhookendpoints

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the signing secrets of the Webhook endpoints of every Workspace, as the Secret
// re-encryption sweeps them (secrets.Cipher.Reencrypt). They read and swap on the raw client, and
// use it for nothing else (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: webhookendpoint.Table + "." + webhookendpoint.FieldSecretEncrypted,
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.WebhookEndpoint.Query().Where(webhookendpoint.IDGT(after)).Order(ent.Asc(webhookendpoint.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.WebhookEndpoint, _ int) secrets.Stored {
				return secrets.Stored{ID: e.ID, Value: e.SecretEncrypted}
			}), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.WebhookEndpoint.Update().Where(webhookendpoint.ID(id), webhookendpoint.SecretEncrypted(prev)).SetSecretEncrypted(next).Save(ctx)
			return n > 0, err
		},
	}}
}
