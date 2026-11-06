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

// Secrets are the sealed fields of the http Actions of every Rule version of every Workspace:
// every version, the draft and those of a deleted Rule too, since a Firing reads its version. It
// holds the raw client, which it uses only to re-encrypt them (docs/architecture.md, Workspace
// scoping).
type Secrets struct {
	client *ent.Client
}

// NewSecrets builds the sealed fields of the Rule versions on the raw client.
func NewSecrets(client *ent.Client) *Secrets { return &Secrets{client: client} }

// Reencrypt stores every version whose http Actions hold a field sealed under an older key again,
// each field under the primary key of cipher, by id and compare-and-swap of the whole Actions
// (secrets.Cipher.Reencrypt).
func (s *Secrets) Reencrypt(ctx context.Context, cipher *secrets.Cipher) ([]secrets.Count, error) {
	c := s.client
	return cipher.Reencrypt(ctx, secrets.Sweep{
		Column: "rule_versions.actions",
		Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
			rs, err := c.RuleVersion.Query().Where(ruleversion.IDGT(after)).Order(ent.Asc(ruleversion.FieldID)).Limit(n).All(ctx)
			return lo.Map(rs, func(e *ent.RuleVersion, _ int) secrets.Stored { return secrets.Stored{ID: e.ID, Value: e.Actions} }), err
		},
		Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
			n, err := c.RuleVersion.Update().Where(ruleversion.ID(id), ruleversion.Actions(prev)).SetActions(next).Save(ctx)
			return n > 0, err
		},
		Seal: func(stored string) (string, bool, error) { return rewrapActions(cipher, stored) },
	})
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
