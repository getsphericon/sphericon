package rules_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// evaluate hands one Event to the module as the evaluation job does, with no queue: the Actions
// jobs it writes sit in the test's queue.
func evaluate(t *testing.T, env *testhelper.TestEnv, contactID int64, action, sourceID string) []int64 {
	t.Helper()
	m := rules.New(env.Bus)
	fired, err := m.Evaluate(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.Event{
		ContactID:  contactID,
		Action:     action,
		SourceID:   sourceID,
		RecordedAt: time.Now(),
	})
	require.NoError(t, err)
	ruleIDs := make([]int64, 0, len(fired))
	for _, f := range fired {
		ruleIDs = append(ruleIDs, f.RuleID)
	}
	return ruleIDs
}

func TestEvaluateFiresTheActiveRuleOfTheTriggerEvent(t *testing.T) {
	env := testhelper.Setup(t)

	fired := evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-1")

	// Globex's Rule has the same Trigger event and is not Acme's.
	assert.Equal(t, []int64{fixtures.RuleTagClickedID}, fired)
	firing := env.DB.RuleFiring.Query().
		Where(rulefiring.RuleID(fixtures.RuleTagClickedID), rulefiring.SourceID("evt-1")).
		OnlyX(context.Background())
	assert.EqualValues(t, fixtures.ContactAliceID, firing.ContactID)
	assert.Equal(t, "rule_click", firing.Action)
	assert.Nil(t, firing.CompletedAt, "the Actions run on a job of their own")
}

func TestEvaluateQueuesTheActionsJobOfEachFiring(t *testing.T) {
	env := testhelper.Setup(t)
	before := len(env.JobsOf(t, rules.ActionsKind))

	evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-queue")

	queued := env.JobsOf(t, rules.ActionsKind)
	assert.Len(t, queued, before+1)
}

func TestEvaluateFiresAnyOneOfSeveralTriggerEvents(t *testing.T) {
	env := testhelper.Setup(t)

	first := evaluate(t, env, fixtures.ContactAliceID, "rule_multi_a", "evt-a")
	second := evaluate(t, env, fixtures.ContactAliceID, "rule_multi_b", "evt-b")

	assert.Equal(t, []int64{fixtures.RuleMultiTriggerID}, first)
	assert.Equal(t, []int64{fixtures.RuleMultiTriggerID}, second)
}

func TestEvaluateFiresAnEventDeliveredTwiceOnce(t *testing.T) {
	env := testhelper.Setup(t)

	first := evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-twice")
	again := evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-twice")

	assert.Equal(t, []int64{fixtures.RuleTagClickedID}, first)
	assert.Empty(t, again)
	firings := env.DB.RuleFiring.Query().
		Where(rulefiring.RuleID(fixtures.RuleTagClickedID), rulefiring.SourceID("evt-twice")).
		AllX(context.Background())
	assert.Len(t, firings, 1)
	queued := env.JobsOf(t, rules.ActionsKind)
	assert.Len(t, queued, 1, "the second delivery queued nothing")
}

func TestEvaluateFiresEveryDistinctEvent(t *testing.T) {
	env := testhelper.Setup(t)

	first := evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-one")
	second := evaluate(t, env, fixtures.ContactAliceID, "rule_click", "evt-two")

	assert.Equal(t, []int64{fixtures.RuleTagClickedID}, first)
	assert.Equal(t, []int64{fixtures.RuleTagClickedID}, second)
}

func TestEvaluateLeavesNothingWhenTheConditionIsFalse(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleVipOnly needs the vip Tag: Alice has it, Bob does not.
	forAlice := evaluate(t, env, fixtures.ContactAliceID, "rule_vip", "evt-vip-alice")
	forBob := evaluate(t, env, fixtures.ContactBobID, "rule_vip", "evt-vip-bob")

	assert.Equal(t, []int64{fixtures.RuleVipOnlyID}, forAlice)
	assert.Empty(t, forBob)
	bobs := env.DB.RuleFiring.Query().Where(rulefiring.ContactID(fixtures.ContactBobID), rulefiring.SourceID("evt-vip-bob")).AllX(context.Background())
	assert.Empty(t, bobs)
}

func TestEvaluateIgnoresADraftAndAnEventNoRuleTriggersOn(t *testing.T) {
	env := testhelper.Setup(t)

	draft := evaluate(t, env, fixtures.ContactAliceID, "rule_draft", "evt-draft")
	unknown := evaluate(t, env, fixtures.ContactAliceID, "nobody_listens", "evt-unknown")

	assert.Empty(t, draft)
	assert.Empty(t, unknown)
}

func TestEvaluateNeverReplaysAnEventRecordedBeforeActivation(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleActivatedLater was activated an hour from now: an Event recorded now predates it.
	before := evaluate(t, env, fixtures.ContactAliceID, "rule_later", "evt-before")

	m := rules.New(env.Bus)
	after, err := m.Evaluate(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.Event{
		ContactID: fixtures.ContactAliceID, Action: "rule_later", SourceID: "evt-after", RecordedAt: time.Now().Add(2 * time.Hour),
	})
	require.NoError(t, err)

	assert.Empty(t, before)
	require.Len(t, after, 1)
	assert.EqualValues(t, fixtures.RuleActivatedLaterID, after[0].RuleID)
}

func TestEvaluateFiresNothingForAnotherWorkspacesContact(t *testing.T) {
	env := testhelper.Setup(t)
	m := rules.New(env.Bus)

	// The Contact belongs to Globex; the scope is Acme's.
	fired, err := m.Evaluate(context.Background(), env.DB.Scoped(fixtures.AcmeID), rules.Event{
		ContactID: fixtures.ContactGlobexID, Action: "rule_click", SourceID: "evt-foreign", RecordedAt: time.Now(),
	})

	require.NoError(t, err)
	assert.Empty(t, fired)
}
