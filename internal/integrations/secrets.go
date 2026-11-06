package integrations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the stored secrets of the Integrations of every Workspace, the discarded ones
// included: the provider config and the inbound params. It holds the raw client, which it uses
// only to re-encrypt them (docs/architecture.md, Workspace scoping).
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the stored secrets of the Integrations on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every config and params value sealed under an older key again under the
// primary key of cipher, by id and compare-and-swap (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	page := func(of func(*ent.Integration) string) func(context.Context, int64, int) ([]secrets.Stored, error) {
		return func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.Integration.Query().Where(integration.IDGT(after)).Order(ent.Asc(integration.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.Integration, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: of(e)} }), err
		}
	}
	return cipher.Reencrypt(ctx,
		secrets.Sweep{
			Column: "integrations.config_encrypted",
			Page:   page(func(e *ent.Integration) string { return e.ConfigEncrypted }),
			Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ConfigEncrypted(prev)).SetConfigEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		secrets.Sweep{
			Column: "integrations.params_encrypted",
			Page:   page(func(e *ent.Integration) string { return e.ParamsEncrypted }),
			Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ParamsEncrypted(prev)).SetParamsEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
	)
}
