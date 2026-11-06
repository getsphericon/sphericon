package users

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the Second factor secrets of every User. It holds the raw client, which it uses only
// to re-encrypt them.
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the Second factor secrets of the Users on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every Second factor secret sealed under an older key again under the primary
// key of cipher, by id and compare-and-swap (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
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
	})
}
