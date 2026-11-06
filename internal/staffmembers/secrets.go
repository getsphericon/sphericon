package staffmembers

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the TOTP secrets of every Staff member. It holds the raw client, which it uses only
// to re-encrypt them.
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the TOTP secrets of the Staff members on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every TOTP secret sealed under an older key again under the primary key of
// cipher, by id and compare-and-swap (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
		Column: "staff_members.totp_secret_encrypted",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.StaffMember.Query().Where(staffmember.IDGT(after)).Order(ent.Asc(staffmember.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.StaffMember, _ int) secrets.Stored {
				return secrets.Stored{ID: e.ID, Value: e.TotpSecretEncrypted}
			}), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.StaffMember.Update().Where(staffmember.ID(id), staffmember.TotpSecretEncrypted(prev)).SetTotpSecretEncrypted(next).Save(ctx)
			return n > 0, err
		},
	})
}
