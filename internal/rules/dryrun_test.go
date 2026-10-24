package rules_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func dryRun(t *testing.T, env *testhelper.TestEnv, ruleID, eventID int64) rules.DryRun {
	t.Helper()
	res, err := rules.New(env.Cipher, env.Bus).DryRun(context.Background(), env.DB.Scoped(fixtures.AcmeID), ruleID, eventID)
	require.NoError(t, err)
	return res
}

func TestDryRunReportsTheActionsAStoredEventWouldRun(t *testing.T) {
	env := testhelper.Setup(t)

	res := dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickLiamID)

	assert.Equal(t, rules.WouldFire, res.Outcome)
	require.Len(t, res.Actions, 1)
	assert.Equal(t, rules.ActionApplyTag, res.Actions[0].Type)
	assert.Contains(t, res.Actions[0].Summary, "rule-clicked")
}

func TestDryRunWritesAndRunsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	firings := env.DB.RuleFiring.Query().CountX(ctx)
	tags := env.DB.ContactTag.Query().CountX(ctx)
	jobs := len(env.JobsOf(t, rules.ActionsKind))

	dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickLiamID)
	dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickLiamID)

	assert.Equal(t, firings, env.DB.RuleFiring.Query().CountX(ctx), "no Firing")
	assert.Equal(t, tags, env.DB.ContactTag.Query().CountX(ctx), "no Tag change")
	assert.Len(t, env.JobsOf(t, rules.ActionsKind), jobs, "no Actions job queued")
}

func TestDryRunSaysWhyARuleWouldNotFire(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleVipOnly needs the vip Tag, which Liam lacks; the Event has no Contact; the Rule has
	// another Trigger event.
	assert.Equal(t, rules.ConditionNotMet, dryRun(t, env, fixtures.RuleVipOnlyID, fixtures.EventRuleVipLiamID).Outcome)
	assert.Equal(t, rules.NoContact, dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickAnonymousID).Outcome)
	assert.Equal(t, rules.NotTriggered, dryRun(t, env, fixtures.RuleVipOnlyID, fixtures.EventRuleClickLiamID).Outcome)
	assert.Empty(t, dryRun(t, env, fixtures.RuleVipOnlyID, fixtures.EventRuleVipLiamID).Actions)
}

// A mail scanner's open or click never fires a Rule, so a dry run on one must not say it would.
func TestDryRunSaysAMachineEventNeverFires(t *testing.T) {
	env := testhelper.Setup(t)

	res := dryRun(t, env, fixtures.RuleTagClickedID, fixtures.EventRuleClickMachineID)

	assert.Equal(t, rules.Machine, res.Outcome)
	assert.Empty(t, res.Actions)
}

func TestDryRunTestsTheStoredEventsProperties(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, rules.WouldFire, dryRun(t, env, fixtures.RuleUrlPricingID, fixtures.EventRuleUrlPricingID).Outcome)
	assert.Equal(t, rules.ConditionNotMet, dryRun(t, env, fixtures.RuleUrlPricingID, fixtures.EventRuleUrlDocsID).Outcome)
}

// An Event too heavy to be shown to a Rule has no properties for a real firing; a dry run must
// say the same, not "would fire" for an Event the Rule would never fire on.
func TestDryRunSeesAnOversizedEventsPropertiesAsAbsent(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, rules.ConditionNotMet, dryRun(t, env, fixtures.RuleUrlPricingID, fixtures.EventRuleUrlOversizedID).Outcome)
}

func TestDryRunTestsADraftWithoutLookingAtActivation(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleDraft is a draft: dry-running it shows what activating it would do.
	res := dryRun(t, env, fixtures.RuleDraftID, fixtures.EventRuleClickLiamID)

	assert.Equal(t, rules.NotTriggered, res.Outcome, "rule_click is not the draft's Trigger event")
}

func TestDryRunRefusesAnotherWorkspacesEventOrRule(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Cipher, env.Bus)
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := m.DryRun(context.Background(), s, fixtures.RuleTagClickedID, fixtures.EventGlobexID)
	require.ErrorIs(t, err, rules.ErrEventNotFound)
	_, err = m.DryRun(context.Background(), s, fixtures.RuleGlobexID, fixtures.EventRuleClickLiamID)
	assert.ErrorIs(t, err, rules.ErrNotFound)
}
