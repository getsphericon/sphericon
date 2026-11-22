package automations

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/infra/events"
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
func guardedSteps(c *ent.Client, id int64) secrets.Guarded[automationstep.Path] {
	var owner, workspace int64
	return secrets.Guarded[automationstep.Path]{
		// The Automation is read first: an edit between the two reads leaves an older revision with
		// the edited steps, and the write misses.
		Read: func(ctx context.Context) (automationstep.Path, int64, error) {
			a, err := c.Automation.Query().Where(automation.HasVersionsWith(automationversion.ID(id))).Only(ctx)
			if ent.IsNotFound(err) {
				return automationstep.Path{}, 0, secrets.ErrRowGone
			}
			if err != nil {
				return automationstep.Path{}, 0, err
			}
			owner, workspace = a.ID, a.WorkspaceID
			v, err := c.AutomationVersion.Get(ctx, id)
			if ent.IsNotFound(err) {
				return automationstep.Path{}, 0, secrets.ErrRowGone
			}
			if err != nil {
				return automationstep.Path{}, 0, err
			}
			return v.Steps, a.Revision, nil
		},
		Seal: rewrapSteps,
		// An edit holds the Workspace's lock from its read of the version to its write, and writes
		// back the secrets it read: the write takes the lock or leaves the version to the next run,
		// and under it lands only while the Automation has the revision read.
		Write: func(ctx context.Context, steps automationstep.Path, revision int64) (bool, error) {
			tx, err := c.Tx(ctx)
			if err != nil {
				return false, err
			}
			defer func() { _ = tx.Rollback() }()
			if took, err := events.TryLockWorkspace(ctx, tx.Client(), workspace); err != nil || !took {
				return false, err
			}
			if held, err := tx.Automation.Query().Where(automation.ID(owner), automation.Revision(revision)).Exist(ctx); err != nil || !held {
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
func rewrapSteps(cipher *secrets.Cipher, steps automationstep.Path) (sealed automationstep.Path, changed bool, err error) {
	sealed, err = automationstep.Map(steps, func(_ int, st *automationstep.Step) (automationstep.Step, error) {
		h, ok := st.Payload.(automationstep.HTTP)
		if !ok {
			return *st, nil
		}
		h.Headers = slices.Clone(h.Headers)
		did, err := httpcall.Rewrap(cipher, &h.URL, &h.Secret, h.Headers)
		if err != nil {
			return automationstep.Step{}, err
		}
		changed = changed || did
		return automationstep.Step{ID: st.ID, Payload: h}, nil
	})
	if err != nil {
		return automationstep.Path{}, false, err
	}
	return sealed, changed, nil
}
