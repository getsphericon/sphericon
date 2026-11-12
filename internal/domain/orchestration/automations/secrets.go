package automations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// SecretSweeps are the sealed fields of the HTTP steps of every Automation version of every
// Workspace: every version, not only the latest, since an Enrollment still stands on an old one;
// each field is sealed on its own, and a version is swapped whole, as the Secret re-encryption
// sweeps them (secrets.Cipher.Reencrypt). They read and swap on the raw client, and use it for
// nothing else (docs/architecture/workspace-scoping.md, Workspace scoping).
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	return []secrets.Sweep{{
		Column: automationversion.Table + "." + automationversion.FieldSteps,
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.AutomationVersion.Query().Where(automationversion.IDGT(after)).Order(ent.Asc(automationversion.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.AutomationVersion, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: e.Steps} }), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.AutomationVersion.Update().Where(automationversion.ID(id), automationversion.Steps(prev)).SetSteps(next).Save(ctx)
			return n > 0, err
		},
		Seal: rewrapSteps,
	}}
}

// rewrapSteps rewraps the sealed fields of an Automation version's HTTP steps: the URL, the header
// values and the signing secret, each sealed on its own (ADR 0039).
func rewrapSteps(cipher *secrets.Cipher, stored string) (sealed string, changed bool, err error) {
	steps, err := automationstep.Decode(stored)
	if err != nil {
		return "", false, err
	}
	for i := range steps {
		if steps[i].Type != automationstep.HTTP {
			continue
		}
		did, err := httpcall.Rewrap(cipher, &steps[i].URL, &steps[i].Secret, steps[i].Headers)
		if err != nil {
			return "", false, err
		}
		changed = changed || did
	}
	if !changed {
		return stored, false, nil
	}
	next, err := Encode(steps)
	return next, true, err
}
