package commandline_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/ruleversion"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// raceOnRead runs edit once, right after the raw client has read the one row of id alone: the
// moment a rotation holds a version it is about to reseal and write back.
func raceOnRead[V any](id func(V) int64, target int64, edit func()) ent.Interceptor {
	raced := false
	return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			v, err := next.Query(ctx, q)
			if rows, ok := v.([]V); ok && len(rows) == 1 && id(rows[0]) == target && !raced {
				raced = true
				edit()
			}
			return v, err
		})
	})
}

// rotated is a cipher of the test keyset with a new primary key, and the one that has retired the
// old key: what reads only a value the rotation sealed again.
func rotated(t *testing.T) (rotation, retired *secrets.Cipher) {
	t.Helper()
	cfg, err := config.Load(config.EnvTest)
	require.NoError(t, err)
	rotatedKeyset, err := secrets.Rotate(cfg.EncryptionKey)
	require.NoError(t, err)
	retiredKeyset, err := secrets.Retire(rotatedKeyset)
	require.NoError(t, err)
	return keysetCipher(t, rotatedKeyset), keysetCipher(t, retiredKeyset)
}

// A Rule edited while the rotation holds its draft (fixture, sealed under the old key): the
// rotation's write misses the Rule's revision, so it reads the draft again and seals that (ADR
// 0169): the edit stays, and its secrets are under the new key.
func TestReencryptRacingARuleEditKeepsTheEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	module := testhelper.Module[*rules.Module](env)
	draft := env.DB.RuleVersion.GetX(ctx, fixtures.RuleSealedDraftVersionID)
	rotation, retired := rotated(t)
	env.DB.RuleVersion.Intercept(raceOnRead(func(v *ent.RuleVersion) int64 { return v.ID }, draft.ID, func() {
		_, err := module.Update(ctx, acme, fixtures.RuleSealedDraftID, rules.UpdateInput{Revision: testhelper.RuleRevisionIn(ctx, t, acme, fixtures.RuleSealedDraftID), Actions: &[]ruleaction.Action{{
			Type: ruleaction.HTTP, ID: draft.Actions[0].ID, Body: `{"v":2}`,
		}}})
		require.NoError(t, err)
	}))

	_, err := reencrypt(ctx, env, rotation, &bytes.Buffer{})
	require.NoError(t, err)

	stored := env.DB.RuleVersion.GetX(ctx, draft.ID).Actions
	require.Len(t, stored, 1)
	assert.JSONEq(t, `{"v":2}`, stored[0].Body, "the edit stays")
	opened, err := rules.New(retired, env.Bus, nil, time.Now).OpenHTTP(stored[0])
	require.NoError(t, err, "the edited draft is sealed under the new key")
	assert.Equal(t, "https://hooks.example.com/sealed", opened.URL)
}

// An Automation edited while the rotation holds its draft: as for a Rule, the edit stays and is
// sealed under the new key.
func TestReencryptRacingAnAutomationEditKeepsTheEdit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	module := testhelper.Module[*automations.Module](env)
	draft := env.DB.AutomationVersion.GetX(ctx, fixtures.AutomationSealedDraftVersionID)
	rotation, retired := rotated(t)
	env.DB.AutomationVersion.Intercept(raceOnRead(func(v *ent.AutomationVersion) int64 { return v.ID }, draft.ID, func() {
		_, err := module.Update(ctx, acme, fixtures.AutomationSealedDraftID, automations.UpdateInput{Revision: env.AutomationRevision(ctx, t, fixtures.AutomationSealedDraftID), Steps: new(automationstep.Root(automationstep.Step{
			ID: draft.Steps.Steps[0].ID, Payload: automationstep.HTTP{Body: `{"v":2}`},
		}))})
		require.NoError(t, err)
	}))

	_, err := reencrypt(ctx, env, rotation, &bytes.Buffer{})
	require.NoError(t, err)

	stored := env.DB.AutomationVersion.GetX(ctx, draft.ID).Steps.Steps
	require.Len(t, stored, 1)
	call, ok := stored[0].Payload.(automationstep.HTTP)
	require.True(t, ok)
	assert.JSONEq(t, `{"v":2}`, call.Body, "the edit stays")
	url, err := retired.Decrypt(call.URL)
	require.NoError(t, err, "the edited draft is sealed under the new key")
	assert.Equal(t, "https://hooks.example.com/sealed", string(url))
}

// A draft dropped by a Revert while the rotation holds its Rule leaves nothing to seal: the run
// goes on to the other versions instead of stopping on the missing row.
func TestReencryptGoesOnPastADraftRevertedMeanwhile(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	module := testhelper.Module[*rules.Module](env)
	rotation, retired := rotated(t)
	env.DB.Rule.Intercept(raceOnRead(func(r *ent.Rule) int64 { return r.ID }, fixtures.RuleTagClickedID, func() {
		_, err := module.Revert(ctx, acme, fixtures.RuleTagClickedID, testhelper.RuleRevisionIn(ctx, t, acme, fixtures.RuleTagClickedID))
		require.NoError(t, err)
	}))

	_, err := reencrypt(ctx, env, rotation, &bytes.Buffer{})

	require.NoError(t, err)
	assert.False(t, env.DB.RuleVersion.Query().Where(ruleversion.ID(fixtures.RuleTagClickedDraftVersionID)).ExistX(ctx), "the draft was reverted")
	sealed := env.DB.RuleVersion.GetX(ctx, fixtures.RuleSealedDraftVersionID).Actions
	_, err = retired.Decrypt(sealed[0].URL)
	require.NoError(t, err, "a version after the reverted one is sealed under the new key")
}

// A rotation run while an edit holds the Rule's draft, read and not yet written: the edit writes
// back the secrets it read, so the rotation does not claim the draft under the Workspace's lock the
// edit holds. It leaves it to the next run, which seals the edited draft (ADR 0169).
func TestReencryptDuringAnEditLeavesTheDraftToTheNextRun(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed()) // the edit and the rotation hold transactions of their own
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	module := testhelper.Module[*rules.Module](env)
	draft := env.DB.RuleVersion.GetX(ctx, fixtures.RuleSealedDraftVersionID)
	revision := testhelper.RuleRevisionIn(ctx, t, acme, fixtures.RuleSealedDraftID)
	rotation, retired := rotated(t)
	var during []secrets.Count
	// The edit has read the draft under the Workspace's lock and is about to write it.
	raced := false
	env.Bus.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if v, ok := m.(*ent.RuleVersionMutation); ok && !raced {
				if id, ok := v.ID(); ok && id == draft.ID {
					raced = true
					var err error
					during, err = reencrypt(ctx, env, rotation, &bytes.Buffer{})
					require.NoError(t, err)
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	_, err := module.Update(ctx, acme, fixtures.RuleSealedDraftID, rules.UpdateInput{Revision: revision, Actions: &[]ruleaction.Action{{
		Type: ruleaction.HTTP, ID: draft.Actions[0].ID, Body: `{"v":2}`,
	}}})
	require.NoError(t, err)

	column := ruleversion.Table + "." + ruleversion.FieldActions
	require.True(t, raced, "the rotation ran inside the edit")
	left := lo.SumBy(during, func(c secrets.Count) int { return lo.Ternary(c.Column == column, c.Raced, 0) })
	assert.Positive(t, left, "the draft is left to the next run")
	_, err = reencrypt(ctx, env, rotation, &bytes.Buffer{})
	require.NoError(t, err)
	stored := env.DB.RuleVersion.GetX(ctx, draft.ID).Actions
	assert.JSONEq(t, `{"v":2}`, stored[0].Body, "the edit stays")
	_, err = retired.Decrypt(stored[0].URL)
	require.NoError(t, err, "the next run seals the edited draft")
}
