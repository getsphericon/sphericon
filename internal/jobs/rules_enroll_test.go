package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestRuleActionEnrollsTheContactInAnAutomation(t *testing.T) {
	env := testhelper.Setup(t)

	f := runFiring(t, env, fixtures.RuleFiringEnrollGreetingPendingID)

	assert.Equal(t, []rules.Result{{Type: rules.ActionEnroll, Status: rules.Succeeded}}, resultsOf(t, f))
	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactBobID)).
		Only(context.Background())
	require.NoError(t, err)
	assert.Contains(t, env.JobsOf(t, jobkind.RunStep.Kind), stepJob(run.ID, 0), "the run starts stepping")
}

func TestRuleActionSkipsAnAutomationThatIsNotActive(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).Automation().Archive(context.Background(), fixtures.AutomationJobsSingleEmailID))

	draft := runFiring(t, env, fixtures.RuleFiringEnrollDraftPendingID)
	archived := runFiring(t, env, fixtures.RuleFiringEnrollSingleEmailPendingID)

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

// The Rule job reads the Automation before the Enrollment's lock: one deactivated since enrolls
// nobody, because the Enrollment checks it again under that lock, as an Enrollment from a Trigger
// does (ADR 0099).
func TestAnAutomationDeactivatedAfterTheRuleJobsCheckEnrollsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule()).
		Deactivate(ctx, s, fixtures.AutomationJobsGreetingID)
	require.NoError(t, err)

	made, err := stepping.EnrollOnRule(ctx, env.Bus, s, fixtures.AutomationJobsGreetingID, fixtures.ContactBobID, "evt-deactivated")

	require.ErrorIs(t, err, automations.ErrInactive)
	assert.False(t, made)
	enrolled := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactBobID)).
		ExistX(ctx)
	assert.False(t, enrolled)
}

func TestRuleActionSkipsAContactAlreadyEnrolled(t *testing.T) {
	env := testhelper.Setup(t)
	greeting := func() int {
		return env.DB.AutomationRun.Query().Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID)).CountX(context.Background())
	}
	before := greeting()

	// Carol was enrolled in the greeting before: it is once ever, even though that run is over.
	f := runFiring(t, env, fixtures.RuleFiringEnrollGreetingCarolID)

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
