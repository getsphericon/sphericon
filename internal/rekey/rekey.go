// Package rekey re-encrypts every secret stored in the database under the primary key of the
// encryption keyset, so a key an operator rotated away from can be retired. It reads and writes
// rows of every Workspace (and the User and Staff member rows no Workspace owns), so it holds
// the raw client; it is the only place that sweeps the stored ciphertext.
package rekey

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// pageSize is how many rows a sweep loads at once.
const pageSize = 200

// Count is what one sweep did to a column.
type Count struct {
	// Column names the stored secret, as "table.column".
	Column string
	// Rewritten values were sealed under an older key and now are under the primary one.
	Rewritten int
	// Current values were already under the primary key and were left alone.
	Current int
	// Raced rows changed while the sweep ran (a request saved them): their new value was
	// sealed by the running server, so they are skipped. A second run counts them as current.
	Raced int
}

// Module re-encrypts the stored secrets.
type Module struct {
	client *ent.Client
	cipher *secrets.Cipher
}

// New builds the Module on the raw client and the cipher whose primary key is the target.
func New(client *ent.Client, cipher *secrets.Cipher) *Module {
	return &Module{client: client, cipher: cipher}
}

// row is one stored value of a sweep: the row id and what it holds.
type row struct {
	id    int64
	value string
}

// sweep is one column: how to page through it and how to write a value back.
type sweep struct {
	column string
	// page returns up to n rows with an id above after, in id order.
	page func(ctx context.Context, after int64, n int) ([]row, error)
	// rewrite compares and swaps: it stores next on the row only while it still holds prev, and
	// reports whether it did.
	rewrite func(ctx context.Context, id int64, prev, next string) (bool, error)
	// seal maps a stored value to the one to store; the default rewraps it whole.
	seal func(value string) (string, bool, error)
}

// Run sweeps every column that holds ciphertext, in id order and page by page. It fails at the
// first value the cipher cannot read, which means a key is missing from the keyset: nothing
// guesses, and what was already rewritten stays valid.
func (m *Module) Run(ctx context.Context) ([]Count, error) {
	sweeps := m.sweeps()
	counts := make([]Count, 0, len(sweeps))
	for _, s := range sweeps {
		c, err := m.run(ctx, s)
		if err != nil {
			return counts, fmt.Errorf("%s: %w", s.column, err)
		}
		counts = append(counts, c)
	}
	return counts, nil
}

func (m *Module) run(ctx context.Context, s sweep) (Count, error) {
	c := Count{Column: s.column}
	seal := s.seal
	if seal == nil {
		seal = m.cipher.Rewrap
	}
	var after int64
	for {
		rows, err := s.page(ctx, after, pageSize)
		if err != nil {
			return c, err
		}
		for _, r := range rows {
			after = r.id
			if r.value == "" {
				continue
			}
			next, changed, err := seal(r.value)
			if err != nil {
				return c, fmt.Errorf("row %d: %w", r.id, err)
			}
			if !changed {
				c.Current++
				continue
			}
			swapped, err := s.rewrite(ctx, r.id, r.value, next)
			if err != nil {
				return c, fmt.Errorf("row %d: %w", r.id, err)
			}
			if swapped {
				c.Rewritten++
			} else {
				c.Raced++
			}
		}
		if len(rows) < pageSize {
			return c, nil
		}
	}
}

func (m *Module) sweeps() []sweep {
	c := m.client
	return []sweep{
		{
			column: "integrations.config_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.Integration.Query().Where(integration.IDGT(after)).Order(ent.Asc(integration.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.Integration) row { return row{e.ID, e.ConfigEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ConfigEncrypted(prev)).SetConfigEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			column: "integrations.params_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.Integration.Query().Where(integration.IDGT(after)).Order(ent.Asc(integration.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.Integration) row { return row{e.ID, e.ParamsEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.Integration.Update().Where(integration.ID(id), integration.ParamsEncrypted(prev)).SetParamsEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			column: "webhook_endpoints.secret_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.WebhookEndpoint.Query().Where(webhookendpoint.IDGT(after)).Order(ent.Asc(webhookendpoint.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.WebhookEndpoint) row { return row{e.ID, e.SecretEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.WebhookEndpoint.Update().Where(webhookendpoint.ID(id), webhookendpoint.SecretEncrypted(prev)).SetSecretEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			column: "sending_domains.dkim_private_key_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.SendingDomain.Query().Where(sendingdomain.IDGT(after)).Order(ent.Asc(sendingdomain.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.SendingDomain) row { return row{e.ID, e.DkimPrivateKeyEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.SendingDomain.Update().Where(sendingdomain.ID(id), sendingdomain.DkimPrivateKeyEncrypted(prev)).SetDkimPrivateKeyEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			column: "users.second_factor_secret_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.User.Query().Where(user.IDGT(after)).Order(ent.Asc(user.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.User) row { return row{e.ID, e.SecondFactorSecretEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.User.Update().Where(user.ID(id), user.SecondFactorSecretEncrypted(prev)).SetSecondFactorSecretEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			column: "staff_members.totp_secret_encrypted",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.StaffMember.Query().Where(staffmember.IDGT(after)).Order(ent.Asc(staffmember.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.StaffMember) row { return row{e.ID, e.TotpSecretEncrypted} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.StaffMember.Update().Where(staffmember.ID(id), staffmember.TotpSecretEncrypted(prev)).SetTotpSecretEncrypted(next).Save(ctx)
				return n > 0, err
			},
		},
		{
			// Every version, not only the latest: an Enrollment still stands on an old one.
			column: "automation_versions.steps",
			page: func(ctx context.Context, after int64, n int) ([]row, error) {
				rs, err := c.AutomationVersion.Query().Where(automationversion.IDGT(after)).Order(ent.Asc(automationversion.FieldID)).Limit(n).All(ctx)
				return rows(rs, func(e *ent.AutomationVersion) row { return row{e.ID, e.Steps} }), err
			},
			rewrite: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := c.AutomationVersion.Update().Where(automationversion.ID(id), automationversion.Steps(prev)).SetSteps(next).Save(ctx)
				return n > 0, err
			},
			seal: m.sealSteps,
		},
	}
}

// sealSteps rewraps the sealed fields of an Automation version's HTTP steps: the URL, the header
// values and the signing secret, each sealed on its own (ADR 0039).
func (m *Module) sealSteps(stored string) (string, bool, error) {
	steps, err := automations.Decode(stored)
	if err != nil {
		return "", false, err
	}
	var changed bool
	rewrap := func(v *string) error {
		next, did, err := m.cipher.Rewrap(*v)
		if err != nil || !did {
			return err
		}
		*v, changed = next, true
		return nil
	}
	for i := range steps {
		if steps[i].Type != automations.StepHTTP {
			continue
		}
		fields := []*string{&steps[i].URL, &steps[i].Secret}
		for j := range steps[i].Headers {
			fields = append(fields, &steps[i].Headers[j].Value)
		}
		for _, f := range fields {
			if *f == "" {
				continue
			}
			if err := rewrap(f); err != nil {
				return "", false, err
			}
		}
	}
	if !changed {
		return stored, false, nil
	}
	next, err := automations.Encode(steps)
	return next, true, err
}

func rows[E any](entities []E, of func(E) row) []row {
	out := make([]row, len(entities))
	for i, e := range entities {
		out[i] = of(e)
	}
	return out
}
