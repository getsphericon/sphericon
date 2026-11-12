package rules

import (
	"context"
	"encoding/json"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the sealed fields of the http Actions of every Rule version of every Workspace:
// every version, the draft and those of a deleted Rule too, since a Firing reads its version; each
// field is sealed on its own, and a version is swapped whole, as the Secret re-encryption sweeps
// them (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for nothing else
// (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: ruleversion.Table + "." + ruleversion.FieldActions,
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.RuleVersion.Query().Where(ruleversion.IDGT(after)).Order(ent.Asc(ruleversion.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.RuleVersion, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: e.Actions} }), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.RuleVersion.Update().Where(ruleversion.ID(id), ruleversion.Actions(prev)).SetActions(next).Save(ctx)
			return n > 0, err
		},
		Seal: rewrapActions,
	}}
}

// rewrapActions rewraps the sealed fields of a Rule version's http Actions: the URL, the header
// values and the signing secret, each sealed on its own.
func rewrapActions(cipher *secrets.Cipher, stored string) (sealed string, changed bool, err error) {
	actions, err := DecodeActions(stored)
	if err != nil {
		return "", false, err
	}
	for i := range actions {
		if actions[i].Type != ActionHTTP {
			continue
		}
		did, err := httpcall.Rewrap(cipher, &actions[i].URL, &actions[i].Secret, actions[i].Headers)
		if err != nil {
			return "", false, err
		}
		changed = changed || did
	}
	if !changed {
		return stored, false, nil
	}
	next, err := json.Marshal(actions)
	return string(next), true, err
}
