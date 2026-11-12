package sendingdomains

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the DKIM private keys of the Sending domains of every Workspace, the discarded
// ones included, as the Secret re-encryption sweeps them (secrets.Cipher.Reencrypt). They read and
// swap on the raw client, and use it for nothing else (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: sendingdomain.Table + "." + sendingdomain.FieldDkimPrivateKeyEncrypted,
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
	}}
}
