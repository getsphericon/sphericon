package jobs_test

import (
	"context"
	"testing"
	"time"

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

// A Rule's Enrollment begins when the Event the Rule fired on occurred (#751), not when the
// Actions job runs: the Contact paid after that Event, so the Enrollment is made already at its
// goal, the Action succeeded and no step is queued. It keeps that Event's action and properties
// (#752), as the Rule's own http Action binds them.
func TestARuleEnrollmentBeginsWhenTheEventTheRuleFiredOnOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	fired, err := env.DB.Event.Get(ctx, fixtures.EventRecoveryRuleTriggerID)
	require.NoError(t, err)

	f := fireAndRun(t, env, fixtures.ContactPaidBeforeEnrolledID, fixtures.EventRecoveryRuleTriggerAction, *fired.SourceID)

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactPaidBeforeEnrolledID)).
		Only(ctx)
	require.NoError(t, err)
	results := resultsOf(t, f)
	steps := env.JobsOf(t, jobkind.RunStep.Kind)
	assert.Equal(t, []rules.Result{{Type: rules.ActionEnroll, Status: rules.Succeeded}}, results)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
	assert.WithinDuration(t, fired.OccurredAt, run.BeganAt, time.Microsecond)
	assert.Empty(t, steps, "no step runs after the goal")
	assert.Equal(t, new(fixtures.EventRecoveryRuleTriggerAction), run.EventAction)
	assert.Equal(t, fired.Properties, run.EventProperties)
}
