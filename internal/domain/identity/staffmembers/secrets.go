package staffmembers

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the TOTP secrets of every Staff member, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for nothing else
// (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
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
	}}
}
