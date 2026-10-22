package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// fireAndRun hands the Event to the Rule module, then runs the Actions of the one Firing it wrote,
// as the evaluation job and the Actions job do, with no queue between them.
func fireAndRun(t *testing.T, env *testhelper.TestEnv, contactID int64, action, sourceID string) *ent.RuleFiring {
	t.Helper()
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := rules.New(env.Bus)
	fired, err := m.Evaluate(ctx, s, rules.Event{ContactID: contactID, Action: action, SourceID: sourceID, RecordedAt: time.Now()})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	require.NoError(t, m.RunActions(ctx, s, fired[0].ID, jobs.RuleActionRunners(newMod(env, nil), env.Bus)))
	return env.DB.RuleFiring.GetX(ctx, fired[0].ID)
}

func tagsOf(t *testing.T, env *testhelper.TestEnv, contactID int64) []string {
	t.Helper()
	names, err := env.DB.Tag.Query().
		Where(tag.HasContactsWith(contact.ID(contactID))).
		Order(ent.Asc(tag.FieldName)).
		Select(tag.FieldName).
		Strings(context.Background())
	require.NoError(t, err)
	return names
}

func resultsOf(t *testing.T, f *ent.RuleFiring) []rules.Result {
	t.Helper()
	results, err := rules.DecodeResults(f.Results)
	require.NoError(t, err)
	return results
}

func TestRuleActionAppliesATag(t *testing.T) {
	env := testhelper.Setup(t)

	f := fireAndRun(t, env, fixtures.ContactBobID, "rule_click", "evt-apply")

	assert.Equal(t, []string{"rule-clicked"}, tagsOf(t, env, fixtures.ContactBobID))
	assert.Equal(t, []rules.Result{{Type: rules.ActionApplyTag, Status: rules.Succeeded}}, resultsOf(t, f))
	assert.NotNil(t, f.CompletedAt)
}

func TestRuleActionsRunInOrder(t *testing.T) {
	env := testhelper.Setup(t)

	// RuleMultiTrigger applies rule-multi, then removes vip: Alice starts with vip and newsletter.
	f := fireAndRun(t, env, fixtures.ContactAliceID, "rule_multi_a", "evt-order")

	assert.Equal(t, []string{"newsletter", "rule-multi"}, tagsOf(t, env, fixtures.ContactAliceID))
	assert.Equal(t, []rules.Result{
		{Type: rules.ActionApplyTag, Status: rules.Succeeded},
		{Type: rules.ActionRemoveTag, Status: rules.Succeeded},
	}, resultsOf(t, f))
}

func TestOneFailedRuleActionDoesNotStopTheNext(t *testing.T) {
	env := testhelper.Setup(t)

	f := fireAndRun(t, env, fixtures.ContactBobID, "rule_fail", "evt-fail")

	assert.Equal(t, []string{"rule-after-failure"}, tagsOf(t, env, fixtures.ContactBobID))
	results := resultsOf(t, f)
	require.Len(t, results, 2)
	assert.Equal(t, rules.Failed, results[0].Status)
	assert.NotEmpty(t, results[0].Detail)
	assert.Equal(t, rules.Succeeded, results[1].Status)
	assert.NotNil(t, f.CompletedAt, "a failed Action still lets the Firing finish")
}

func TestRunningAFinishedFiringAgainChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	f := fireAndRun(t, env, fixtures.ContactBobID, "rule_click", "evt-again")
	env.DB.Tag.Delete().Where(tag.Name("rule-clicked")).ExecX(ctx)

	err := rules.New(env.Bus).RunActions(ctx, s, f.ID, jobs.RuleActionRunners(newMod(env, nil), env.Bus))

	require.NoError(t, err)
	assert.Empty(t, tagsOf(t, env, fixtures.ContactBobID), "the Action did not run a second time")
	assert.Len(t, resultsOf(t, env.DB.RuleFiring.GetX(ctx, f.ID)), 1)
}

func TestRunActionsOfAFiringThatIsGoneDoesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	env.DB.RuleFiring.Delete().Where(rulefiring.ID(fixtures.RuleFiringPendingID)).ExecX(context.Background())

	err := rules.New(env.Bus).RunActions(context.Background(), s, fixtures.RuleFiringPendingID, jobs.RuleActionRunners(newMod(env, nil), env.Bus))

	assert.NoError(t, err)
}
