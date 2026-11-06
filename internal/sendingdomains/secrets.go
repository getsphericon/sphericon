package sendingdomains

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the DKIM private keys of the Sending domains of every Workspace, the discarded ones
// included. It holds the raw client, which it uses only to re-encrypt them (docs/architecture.md,
// Workspace scoping).
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the DKIM private keys of the Sending domains on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every DKIM private key sealed under an older key again under the primary key of
// cipher, by id and compare-and-swap (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
		Column: "sending_domains.dkim_private_key_encrypted",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.SendingDomain.Query().Where(sendingdomain.IDGT(after)).Order(ent.Asc(sendingdomain.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.SendingDomain, _ int) secrets.Stored {
				return secrets.Stored{ID: e.ID, Value: e.DkimPrivateKeyEncrypted}
			}), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.SendingDomain.Update().Where(sendingdomain.ID(id), sendingdomain.DkimPrivateKeyEncrypted(prev)).SetDkimPrivateKeyEncrypted(next).Save(ctx)
			return n > 0, err
		},
	})
}
