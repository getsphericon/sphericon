package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/ruleversiontrigger"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/registry"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// queueingClient is the jobs client the consumers hand an Event to, queueing into this test's
// transaction, so what they queue is read back with JobsOf.
func queueingClient(t *testing.T, env *testhelper.TestEnv) *jobs.Client {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	client, err := jobs.NewClient(riverdatabasesql.New(env.SQLDB), env.DB, newMod(env, nil), env.Cipher, env.Bus, env.SystemMail,
		nil, nil, registry.Default(), cfg.AppURL, cfg.OutboundAllow, nil, jobs.Retention{})
	require.NoError(t, err)
	return client
}

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

// onlyEnvelope is the one envelope of the name on the test's outbox.
func onlyEnvelope(t *testing.T, env *testhelper.TestEnv, name string) events.Envelope {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t, name)
	require.Len(t, envelopes, 1)
	return envelopes[0]
}

// An Automation whose Trigger is contact.tagged enrolls a Contact a Tag is applied to (#897), and
// its steps see which Tag it was. Fixture: the Jobs greeting Automation, its live version's
// Trigger turned to contact.tagged.
func TestAnAutomationTriggeredByATagEnrollsTheTaggedContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameContactTagged).ExecX(ctx)

	_, err := tags.New(env.Bus).Apply(ctx, acme, fixtures.ContactLiamID, "onboarding-ready")
	require.NoError(t, err)
	tagged := onlyEnvelope(t, env, events.NameContactTagged)
	require.NoError(t, events.AutomationsConsumer(queueingClient(t, env))(events.Message(tagged)))
	triggers := queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind())
	require.Len(t, triggers, 1)
	require.NoError(t, jobs.NewEvaluateTriggerWorker(env.DB, env.Bus).Work(ctx, job(triggers[0])))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactLiamID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, events.NameContactTagged, *run.EventAction)
	assert.Equal(t, map[string]any{"tag": "onboarding-ready"}, run.EventProperties)
}

// A Rule triggered by automation.completed fires when an Enrollment completes, and what its Action
// then records (the Tag it applies) fires no Rule: Rules never chain (ADR 0054). Fixture: the
// Tag-clicked Rule, which applies rule-clicked, its Trigger turned to automation.completed.
func TestARuleTriggeredByACompletedEnrollmentFiresAndItsTagFiresNoRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	client := queueingClient(t, env)
	env.DB.RuleVersionTrigger.Update().
		Where(ruleversiontrigger.VersionID(fixtures.RuleTagClickedLiveVersionID)).
		SetAction(events.NameAutomationCompleted).
		ExecX(ctx)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	completed := onlyEnvelope(t, env, events.NameAutomationCompleted)
	require.NoError(t, events.RulesConsumer(env.DB, client)(events.Message(completed)))
	evaluations := queuedArgs[jobs.EvaluateRulesArgs](t, env, jobkind.RuleEvaluate.Kind)
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
	require.NoError(t, events.RulesConsumer(env.DB, client)(events.Message(tagged)))
	evaluationsAfter := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	aliceTags := tagsOf(t, env, fixtures.ContactAliceID)

	assert.EqualValues(t, fixtures.RuleTagClickedID, fired[0].RuleID)
	assert.Contains(t, aliceTags, "rule-clicked")
	assert.NotEmpty(t, tagged.CausedBy, "the Action's Tag carries its Rule")
	assert.Len(t, evaluationsAfter, 1, "the Action's own contact.tagged queues no evaluation")
}
