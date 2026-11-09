package users

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the Second factor secrets of every User, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for nothing else
// (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: "users.second_factor_secret_encrypted",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.User.Query().Where(user.IDGT(after)).Order(ent.Asc(user.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.User, _ int) secrets.Stored {
				return secrets.Stored{ID: e.ID, Value: e.SecondFactorSecretEncrypted}
			}), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.User.Update().Where(user.ID(id), user.SecondFactorSecretEncrypted(prev)).SetSecondFactorSecretEncrypted(next).Save(ctx)
			return n > 0, err
		},
	}}
}
