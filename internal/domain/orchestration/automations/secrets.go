package automations

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationsteps"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Secrets are the sealed fields of the HTTP steps of every Automation version of every Workspace:
// every version, not only the latest, since an Enrollment still stands on an old one. It holds the
// raw client, which it uses only to re-encrypt them (docs/architecture.md, Workspace scoping).
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the sealed fields of the Automation versions on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every version whose HTTP steps hold a field sealed under an older key again,
// each field under the primary key of cipher, by id and compare-and-swap of the whole steps
// (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
		Column: "automation_versions.steps",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.AutomationVersion.Query().Where(automationversion.IDGT(after)).Order(ent.Asc(automationversion.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.AutomationVersion, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: e.Steps} }), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.AutomationVersion.Update().Where(automationversion.ID(id), automationversion.Steps(prev)).SetSteps(next).Save(ctx)
			return n > 0, err
		},
		Seal: func(stored string) (string, bool, error) { return rewrapSteps(cipher, stored) },
	})
}

// rewrapSteps rewraps the sealed fields of an Automation version's HTTP steps: the URL, the header
// values and the signing secret, each sealed on its own (ADR 0039).
func rewrapSteps(cipher *secrets.Cipher, stored string) (sealed string, changed bool, err error) {
	steps, err := automationsteps.Decode(stored)
	if err != nil {
		return "", false, err
	}
	for i := range steps {
		if steps[i].Type != automationsteps.HTTP {
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
