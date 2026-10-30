package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestRuleActionEnrollsTheContactInAnAutomation(t *testing.T) {
	env := testhelper.Setup(t)

	f := fireAndRun(t, env, fixtures.ContactBobID, "rule_enroll", "evt-enroll")

	assert.Equal(t, []rules.Result{{Type: rules.ActionEnroll, Status: rules.Succeeded}}, resultsOf(t, f))
	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactBobID)).
		Only(context.Background())
	require.NoError(t, err)
	assert.Contains(t, env.JobsOf(t, jobkind.RunStep.Kind), stepJob(run.ID, 0), "the run starts stepping")
}

func TestRuleActionSkipsAnAutomationThatIsNotActive(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Automation().Discard(context.Background(), fixtures.AutomationJobsSingleEmailID))

	draft := fireAndRun(t, env, fixtures.ContactBobID, "rule_enroll_draft", "evt-draft")
	archived := fireAndRun(t, env, fixtures.ContactBobID, "rule_enroll_single", "evt-archived")

	for _, f := range []*ent.RuleFiring{draft, archived} {
		res := resultsOf(t, f)
		require.Len(t, res, 1)
		assert.Equal(t, rules.Skipped, res[0].Status)
		assert.NotEmpty(t, res[0].Detail)
	}
	enrolled := env.DB.AutomationRun.Query().
		Where(automationrun.ContactID(fixtures.ContactBobID), automationrun.AutomationIDIn(fixtures.AutomationJobsGreetingInactiveID, fixtures.AutomationJobsSingleEmailID)).
		CountX(context.Background())
	assert.Zero(t, enrolled)
}

func TestRuleActionSkipsAContactAlreadyEnrolled(t *testing.T) {
	env := testhelper.Setup(t)
	greeting := func() int {
		return env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID)).CountX(context.Background())
	}
	before := greeting()

	// Carol was enrolled in the greeting before: it is once ever, even though that run is over.
	f := fireAndRun(t, env, fixtures.ContactCarolID, "rule_enroll", "evt-enrolled")

	res := resultsOf(t, f)
	require.Len(t, res, 1)
	assert.Equal(t, rules.Skipped, res[0].Status)
	assert.Equal(t, before, greeting())
	assert.Empty(t, env.JobsOf(t, jobkind.RunStep.Kind))
}
