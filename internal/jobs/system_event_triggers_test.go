package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// queuedArgs are the args of the jobs of kind left on the queue, decoded as T.
func queuedArgs[T any](t *testing.T, env *testhelper.TestEnv, kind string) []T {
	t.Helper()
	queued := env.JobsOf(t, kind)
	out := make([]T, 0, len(queued))
	for _, raw := range queued {
		encoded, err := json.Marshal(raw)
		require.NoError(t, err)
		var args T
		require.NoError(t, json.Unmarshal(encoded, &args))
		out = append(out, args)
	}
	return out
}

// onlyEnvelope is the one envelope of the name the test published.
func onlyEnvelope(t *testing.T, env *testhelper.TestEnv, name string) events.Envelope {
	t.Helper()
	envelopes := env.QueuedEnvelopes(t, name)
	require.Len(t, envelopes, 1)
	return envelopes[0]
}

// An Automation whose Trigger is contact.tagged enrolls a Contact a Tag is applied to (#897), and
// its steps see which Tag it was. Fixture: the Automation the fixtures trigger on contact.tagged.
func TestAnAutomationTriggeredByATagEnrollsTheTaggedContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)

	_, err := contacts.New(env.Bus).ApplyTag(ctx, acme, fixtures.ContactLiamID, "onboarding-ready")
	require.NoError(t, err)
	onlyEnvelope(t, env, events.NameContactTagged)
	triggers := triggersOn(t, env, events.NameContactTagged)
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, testhelper.Enrollments(env.Bus)).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationOnContactTaggedID), automationrun.ContactID(fixtures.ContactLiamID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameContactTagged, *run.EventAction)
	assert.Equal(t, map[string]any{"tag": "onboarding-ready"}, run.EventProperties)
}

// A Rule triggered by automation.completed fires when an Enrollment completes, and what its Action
// then records (the Tag it applies) fires no Rule: Rules never chain (ADR 0054). Fixture: the Rule
// triggered by automation.completed, which applies completed-an-automation.
func TestARuleTriggeredByACompletedEnrollmentFiresAndItsTagFiresNoRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	onlyEnvelope(t, env, events.NameAutomationCompleted)
	evaluations := lo.Filter(queuedArgs[jobs.EvaluateRulesArgs](t, env, jobkind.RuleEvaluate.Kind),
		func(args jobs.EvaluateRulesArgs, _ int) bool { return args.Action == events.NameAutomationCompleted })
	require.Len(t, evaluations, 1)
	m := rules.New(env.Cipher, env.Bus)
	fired, err := m.Evaluate(ctx, acme, rules.Event{
		ContactID: evaluations[0].ContactID, Action: evaluations[0].Action, SourceID: evaluations[0].SourceID,
		RecordedAt: evaluations[0].RecordedAt, Properties: evaluations[0].Properties,
	})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	require.NoError(t, m.RunActions(ctx, acme, fired[0].ID, ruleRunners(env)))
	tagged := onlyEnvelope(t, env, events.NameContactTagged)
	taggedEvaluations := lo.Filter(queuedArgs[jobs.EvaluateRulesArgs](t, env, jobkind.RuleEvaluate.Kind),
		func(args jobs.EvaluateRulesArgs, _ int) bool { return args.Action == events.NameContactTagged })
	aliceTags := tagsOf(t, env, fixtures.ContactAliceID)

	assert.EqualValues(t, fixtures.RuleEnrollmentCompletedID, fired[0].RuleID)
	assert.Contains(t, aliceTags, "completed-an-automation")
	assert.NotEmpty(t, tagged.CausedBy, "the Action's Tag carries its Rule")
	assert.Empty(t, taggedEvaluations, "the Action's own contact.tagged queues no evaluation")
}

// triggerRouter plays river for the Automations: deliver runs the trigger evaluations the Events
// published since its last call queued, and returns their actions.
type triggerRouter struct {
	env       *testhelper.TestEnv
	evaluated int
}

func (r *triggerRouter) deliver(t *testing.T) []string {
	t.Helper()
	triggers := queuedArgs[jobs.EvaluateTriggerArgs](t, r.env, jobs.EvaluateTriggerArgs{}.Kind())
	actions := make([]string, 0, len(triggers)-r.evaluated)
	for _, args := range triggers[r.evaluated:] {
		require.NoError(t, jobs.NewEvaluateTriggerWorker(r.env.DB, testhelper.Enrollments(r.env.Bus)).Work(t.Context(), job(args)))
		actions = append(actions, args.Action)
	}
	r.evaluated = len(triggers)
	return actions
}

// marketing.confirmed is email's consent alone: an Automation triggered on it (a welcome email
// after double opt-in) does not enroll a Contact whose phone's SMS consent is recorded, which is
// sms.confirmed, and does enroll it once its address confirms (ADR 0077). A trigger is an action
// with no property filter, so sharing the action would enroll every phone that consented. Fixture:
// the Automation the fixtures trigger on marketing.confirmed, and the Texter, whose phone and
// address have no Confirmation.
func TestAnAutomationTriggeredByMarketingConfirmedIgnoresAnSMSConfirmation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	router := &triggerRouter{env: env}
	runs := func() []*ent.AutomationRun {
		t.Helper()
		return env.DB.AutomationRun.Query().
			Where(automationrun.AutomationID(fixtures.AutomationOnMarketingConfirmedID), automationrun.ContactID(fixtures.ContactTexterID)).
			AllX(ctx)
	}

	_, err := env.ExternalScoped(t, "contacts:write").ConfirmationsCreate(ctx, &externalapi.CreateConfirmationInput{
		Channel: externalapi.ConsentChannelSMS, Destination: fixtures.ContactTexterPhone,
		Wording: "Text me offers", Source: "checkout form", Brand: "Acme",
	})
	require.NoError(t, err)

	onSMS := router.deliver(t)
	afterSMS := runs()
	require.NoError(t, confirmations.RecordConfirmation(ctx, env.Bus, env.DB.Scoped(fixtures.AcmeID), linktarget.Confirmation{
		Destination: fixtures.ContactTexterEmail, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactTexterID,
	}, "203.0.113.10"))
	onEmail := router.deliver(t)
	afterEmail := runs()

	assert.Equal(t, []string{events.NameSmsConfirmed}, onSMS, "the SMS consent is an Event of the Contact")
	assert.Empty(t, afterSMS, "an SMS Confirmation starts no welcome email")
	assert.Equal(t, []string{events.NameMarketingConfirmed}, onEmail)
	require.Len(t, afterEmail, 1, "the address's confirmation does")
	assert.Equal(t, events.NameMarketingConfirmed, *afterEmail[0].EventAction)
}
