package automations

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the sealed fields of the HTTP steps of every Automation version of every
// Workspace: every version, not only the latest, since an Enrollment still stands on an old one;
// each field is sealed on its own, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt). A version is rewritten whole on the revision of its Automation, which
// every edit of a version moves (ADR 0169): a rewrite that loses to an edit reads the version again.
// They read and write on the raw client, and use it for nothing else
// (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: automationversion.Table + "." + automationversion.FieldSteps,
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			ids, err := c.AutomationVersion.Query().Where(automationversion.IDGT(after)).Order(ent.Asc(automationversion.FieldID)).Limit(n).IDs(ctx)
			return lo.Map(ids, func(id int64, _ int) secrets.Stored { return secrets.Stored{ID: id} }), err
		},
		Rewrite: func(ctx context.Context, cipher *secrets.Cipher, id int64) (secrets.Outcome, error) {
			return guardedSteps(c, id).Rewrite(ctx, cipher)
		},
	}}
}

// guardedSteps is the steps of the Automation version id, written on its Automation's revision.
func guardedSteps(c *ent.Client, id int64) secrets.Guarded[[]automationstep.Step] {
	var owner int64
	return secrets.Guarded[[]automationstep.Step]{
		// The Automation is read first: an edit between the two reads leaves an older revision with
		// the edited steps, and the write misses.
		Read: func(ctx context.Context) ([]automationstep.Step, int64, error) {
			a, err := c.Automation.Query().Where(automation.HasVersionsWith(automationversion.ID(id))).Only(ctx)
			if ent.IsNotFound(err) {
				return nil, 0, secrets.ErrRowGone
			}
			if err != nil {
				return nil, 0, err
			}
			owner = a.ID
			v, err := c.AutomationVersion.Get(ctx, id)
			if ent.IsNotFound(err) {
				return nil, 0, secrets.ErrRowGone
			}
			if err != nil {
				return nil, 0, err
			}
			return v.Steps, a.Revision, nil
		},
		Seal: rewrapSteps,
		// The Automation is locked on the revision read, so an edit either committed before, and
		// the write misses, or waits for it.
		Write: func(ctx context.Context, steps []automationstep.Step, revision int64) (bool, error) {
			tx, err := c.Tx(ctx)
			if err != nil {
				return false, err
			}
			defer func() { _ = tx.Rollback() }()
			held, err := tx.Automation.Query().Where(automation.ID(owner), automation.Revision(revision)).ForUpdate().IDs(ctx)
			if err != nil || len(held) == 0 {
				return false, err
			}
			if err := tx.AutomationVersion.UpdateOneID(id).SetSteps(steps).Exec(ctx); err != nil {
				return false, err
			}
			return true, tx.Commit()
		},
	}
}

// rewrapSteps rewraps the sealed fields of an Automation version's HTTP steps: the URL, the header
// values and the signing secret, each sealed on its own (ADR 0039).
func rewrapSteps(cipher *secrets.Cipher, steps []automationstep.Step) (sealed []automationstep.Step, changed bool, err error) {
	sealed = slices.Clone(steps)
	for i := range sealed {
		if sealed[i].Type != automationstep.HTTP {
			continue
		}
		sealed[i].Headers = slices.Clone(sealed[i].Headers)
		did, err := httpcall.Rewrap(cipher, &sealed[i].URL, &sealed[i].Secret, sealed[i].Headers)
		if err != nil {
			return nil, false, err
		}
		changed = changed || did
	}
	return sealed, changed, nil
}
