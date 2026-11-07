package jobs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// publishIn publishes ev in a bus transaction of its own, in ctx.
func publishIn(ctx context.Context, t *testing.T, env *testhelper.TestEnv, ev events.DomainEvent) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, ev)
	}))
}

// aliceOpened is a person's open of an email by Alice.
func aliceOpened() *events.EmailEngagement {
	return &events.EmailEngagement{
		Action: events.NameEmailOpened, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Email: fixtures.ContactAliceEmail,
	}
}

// A live Event of a Contact queues its evaluation against the Automations, on its semantic action,
// and against the Rules.
func TestALiveEventOfAContactQueuesItsAutomationAndRuleEvaluations(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, aliceOpened())

	triggers := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, triggers, 1)
	assert.EqualValues(t, fixtures.AcmeID, triggers[0]["workspace_id"])
	assert.EqualValues(t, fixtures.ContactAliceID, triggers[0]["contact_id"])
	assert.Equal(t, events.NameEmailOpened, triggers[0]["action"])
	evaluations := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, evaluations, 1)
	assert.EqualValues(t, fixtures.ContactAliceID, evaluations[0]["contact_id"])
	assert.Equal(t, events.NameEmailOpened, evaluations[0]["action"])
}

// When the Event occurred travels with the job: an Event older than an Enrollment ends nothing
// (#751).
func TestTheAutomationEvaluationSaysWhenTheEventOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	occurred := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	publishEvent(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: occurred,
	})

	triggers := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, triggers, 1)
	assert.Equal(t, "2026-03-01T12:00:00Z", triggers[0]["occurred_at"])
}

// The Automation whose email the Event came from travels with the job, so it can refuse to
// enroll the Contact into itself (ADR 0053).
func TestTheAutomationEvaluationNamesTheAutomationOfTheEmail(t *testing.T) {
	env := testhelper.Setup(t)
	opened := aliceOpened()
	opened.AutomationID = fixtures.AutomationOnOpenID
	publishEvent(t, env, opened)

	triggers := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
	require.Len(t, triggers, 1)
	assert.EqualValues(t, fixtures.AutomationOnOpenID, triggers[0]["source_automation_id"])
}

// What is recorded but never reacted to queues no evaluation, against the Automations or the Rules: a
// scanner's open or click (ADR 0053), an Event with no Contact, loaded history and an
// administrative action. An event a Rule's Action published may still enroll, but fires no Rule:
// Rules never chain (ADR 0054).
func TestAnEventThatTriggersNothingQueuesNoAutomationOrRuleJob(t *testing.T) {
	machine := aliceOpened()
	machine.Machine = true
	machineClick := aliceOpened()
	machineClick.Action, machineClick.Machine = events.NameEmailClicked, true
	for name, ev := range map[string]events.DomainEvent{
		"machine open":  machine,
		"machine click": machineClick,
		"no Contact":    &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, SubjectID: "visitor:x", Action: "page_view"},
		"history": &events.CollectedEvent{
			WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "purchase", Backfill: true,
		},
		"audit entry": &events.AuditEntry{WorkspaceID: fixtures.AcmeID, Actor: events.Actor{Kind: events.ActorUser}, Action: "membership.update", TargetType: "membership"},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			publishEvent(t, env, ev)

			automationJobs := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
			ruleJobs := env.JobsOf(t, jobkind.RuleEvaluate.Kind)

			assert.Empty(t, automationJobs)
			assert.Empty(t, ruleJobs)
		})
	}

	t.Run("an Action's own event", func(t *testing.T) {
		env := testhelper.Setup(t)

		publishIn(events.CausedBy(t.Context(), "rule:1"), t, env, aliceOpened())

		automationJobs := env.JobsOf(t, jobkind.EvaluateTrigger.Kind)
		ruleJobs := env.JobsOf(t, jobkind.RuleEvaluate.Kind)

		assert.Len(t, automationJobs, 1)
		assert.Empty(t, ruleJobs)
	})
}

// The rule evaluation carries what a Firing is unique on (the Event's source id: the envelope id,
// as no DedupKey is set) and the properties the Event carries.
func TestTheRuleEvaluationCarriesTheSourceIDAndTheProperties(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click",
		Properties: map[string]any{"url": "https://example.com/a"},
	})
	published := env.QueuedEnvelopes(t)
	require.Len(t, published, 1)

	evaluations := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, evaluations, 1)
	assert.Equal(t, "rule_click", evaluations[0]["action"])
	assert.Equal(t, published[0].ID, evaluations[0]["source_id"])
	assert.Equal(t, map[string]any{"url": "https://example.com/a"}, evaluations[0]["properties"])
}

// An Event heavier than PropertiesBound is evaluated without its properties.
func TestTheRuleEvaluationOfAnOversizedEventHasNoProperties(t *testing.T) {
	env := testhelper.Setup(t)
	publishEvent(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, SubjectID: "alice", Action: "rule_click",
		Properties: map[string]any{"pad": strings.Repeat("x", events.PropertiesBound)},
	})

	evaluations := env.JobsOf(t, jobkind.RuleEvaluate.Kind)
	require.Len(t, evaluations, 1)
	assert.NotContains(t, evaluations[0], "properties")
}

// An evaluation river retries evaluates the Event again, but a Firing is unique on the Event's
// source id, so the Rule fires once. Fixture: RuleSecondRevisit fires on a second rule_revisit
// within a week, and Liam did one yesterday.
func TestARetriedRuleJobFiresTheRuleOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	publishEvent(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactLiamID, SubjectID: "liam", Action: "rule_revisit",
	})

	evaluations := queuedArgs[jobs.EvaluateRulesArgs](t, env, jobkind.RuleEvaluate.Kind)
	require.Len(t, evaluations, 1)
	m := rules.New(env.Cipher, env.Bus, time.Now)
	for _, args := range []jobs.EvaluateRulesArgs{evaluations[0], evaluations[0]} {
		_, err := m.Evaluate(ctx, env.DB.Scoped(fixtures.AcmeID), rules.Event{
			ContactID: args.ContactID, Action: args.Action, SourceID: args.SourceID, RecordedAt: args.RecordedAt, Properties: args.Properties,
		})
		require.NoError(t, err)
	}

	firings, err := env.DB.RuleFiring.Query().
		Where(rulefiring.RuleID(fixtures.RuleSecondRevisitID), rulefiring.ContactID(fixtures.ContactLiamID)).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, firings)
}

// triggersOn are the queued evaluations against the Automations of the Events with action, oldest
// first: every live Event of a Contact queues one when it is published.
func triggersOn(t *testing.T, env *testhelper.TestEnv, action string) []jobs.EvaluateTriggerArgs {
	t.Helper()
	return lo.Filter(queuedArgs[jobs.EvaluateTriggerArgs](t, env, jobs.EvaluateTriggerArgs{}.Kind()),
		func(args jobs.EvaluateTriggerArgs, _ int) bool { return args.Action == action })
}
