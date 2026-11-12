package integrations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the stored secrets of the Integrations of every Workspace, the discarded ones
// included: the provider config and the inbound params, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for nothing else
// (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	page := func(of func(*ent.Integration) string) func(context.Context, int64, int) ([]secrets.Stored, error) {
		return func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.Integration.Query().Where(integration.IDGT(after)).Order(ent.Asc(integration.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.Integration, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: of(e)} }), err
		}
	}
	return []secrets.Sweep{
		{
			Column: integration.Table + "." + integration.FieldConfigEncrypted,
			Page:   page(func(e *ent.Integration) string { return e.ConfigEncrypted }),
			Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ConfigEncrypted(prev)).SetConfigEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			Column: integration.Table + "." + integration.FieldParamsEncrypted,
			Page:   page(func(e *ent.Integration) string { return e.ParamsEncrypted }),
			Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ParamsEncrypted(prev)).SetParamsEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
	}
}
