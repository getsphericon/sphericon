package webhooks

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the signing secrets of the Webhook endpoints of every Workspace. It holds the raw
// client, which it uses only to re-encrypt them (docs/architecture.md, Workspace scoping).
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the signing secrets of the Webhook endpoints on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every signing secret sealed under an older key again under the primary key of
// cipher, by id and compare-and-swap (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
		Column: "webhook_endpoints.secret_encrypted",
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
	})
}
