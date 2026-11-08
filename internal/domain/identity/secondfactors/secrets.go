package secondfactors

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/secondfactor"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the Second factor secrets of every User, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for nothing else
// (docs/architecture.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: "second_factors.secret_encrypted",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.SecondFactor.Query().Where(secondfactor.IDGT(after)).Order(ent.Asc(secondfactor.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.SecondFactor, _ int) secrets.Stored {
				return secrets.Stored{ID: e.ID, Value: e.SecretEncrypted}
			}), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.SecondFactor.Update().Where(secondfactor.ID(id), secondfactor.SecretEncrypted(prev)).SetSecretEncrypted(next).Save(ctx)
			return n > 0, err
		},
	}}
}
