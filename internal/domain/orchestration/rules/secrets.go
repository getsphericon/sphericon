package rules

import (
	"context"
	"slices"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the sealed fields of the http Actions of every Rule version of every Workspace:
// every version, the draft and those of a deleted Rule too, since a Firing reads its version; each
// field is sealed on its own, as the Secret re-encryption sweeps them (secrets.Cipher.Reencrypt). A
// version is rewritten whole on the revision of its Rule, which every edit of a version moves (ADR
// 0169): a rewrite that loses to an edit reads the version again. They read and write on the raw
// client, and use it for nothing else (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: ruleversion.Table + "." + ruleversion.FieldActions,
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			ids, err := c.RuleVersion.Query().Where(ruleversion.IDGT(after)).Order(ent.Asc(ruleversion.FieldID)).Limit(n).IDs(ctx)
			return lo.Map(ids, func(id int64, _ int) secrets.Stored { return secrets.Stored{ID: id} }), err
		},
		Rewrite: func(ctx context.Context, cipher *secrets.Cipher, id int64) (secrets.Outcome, error) {
			return guardedActions(c, id).Rewrite(ctx, cipher)
		},
	}}
}

// guardedActions is the Actions of the Rule version id, written on its Rule's revision.
func guardedActions(c *ent.Client, id int64) secrets.Guarded[[]ruleaction.Action] {
	var owner int64
	return secrets.Guarded[[]ruleaction.Action]{
		// The Rule is read first: an edit between the two reads leaves an older revision with the
		// edited Actions, and the write misses.
		Read: func(ctx context.Context) ([]ruleaction.Action, int64, error) {
			r, err := c.Rule.Query().Where(rule.HasVersionsWith(ruleversion.ID(id))).Only(ctx)
			if ent.IsNotFound(err) {
				return nil, 0, secrets.ErrRowGone
			}
			if err != nil {
				return nil, 0, err
			}
			owner = r.ID
			v, err := c.RuleVersion.Get(ctx, id)
			if ent.IsNotFound(err) {
				return nil, 0, secrets.ErrRowGone
			}
			if err != nil {
				return nil, 0, err
			}
			return v.Actions, r.Revision, nil
		},
		Seal: rewrapActions,
		// The Rule is locked on the revision read, so an edit either committed before, and the
		// write misses, or waits for it.
		Write: func(ctx context.Context, actions []ruleaction.Action, revision int64) (bool, error) {
			tx, err := c.Tx(ctx)
			if err != nil {
				return false, err
			}
			defer func() { _ = tx.Rollback() }()
			held, err := tx.Rule.Query().Where(rule.ID(owner), rule.Revision(revision)).ForUpdate().IDs(ctx)
			if err != nil || len(held) == 0 {
				return false, err
			}
			if err := tx.RuleVersion.UpdateOneID(id).SetActions(actions).Exec(ctx); err != nil {
				return false, err
			}
			return true, tx.Commit()
		},
	}
}

// rewrapActions rewraps the sealed fields of a Rule version's http Actions: the URL, the header
// values and the signing secret, each sealed on its own.
func rewrapActions(cipher *secrets.Cipher, actions []ruleaction.Action) (sealed []ruleaction.Action, changed bool, err error) {
	sealed = slices.Clone(actions)
	for i := range sealed {
		if sealed[i].Type != ruleaction.HTTP {
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
