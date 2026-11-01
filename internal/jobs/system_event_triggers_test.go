package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/ruleversiontrigger"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The consumers' seams, recording what they hand on instead of queueing it.
type handedEnrollments struct{ got []events.AutomationEvent }

func (h *handedEnrollments) OnEvent(_ context.Context, ev events.AutomationEvent) error {
	h.got = append(h.got, ev)
	return nil
}

type handedRuleEvents struct{ got []events.RuleEvent }

func (h *handedRuleEvents) OnRuleEvent(_ context.Context, ev events.RuleEvent) error {
	h.got = append(h.got, ev)
	return nil
}

// onlyEnvelope is the one envelope of the name on the test's outbox.
func onlyEnvelope(t *testing.T, env *testhelper.TestEnv, name string) events.Envelope {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t, name)
	require.Len(t, envelopes, 1)
	return envelopes[0]
}

// An Automation whose Trigger is contact.tagged enrolls a Contact a Tag is applied to (#897).
// Fixture: the Jobs greeting Automation, its live version's Trigger turned to contact.tagged.
func TestAnAutomationTriggeredByATagEnrollsTheTaggedContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	greeting := env.DB.Automation.Query().Where(automation.ID(fixtures.AutomationJobsGreetingID)).OnlyX(ctx)
	env.DB.AutomationVersion.UpdateOneID(*greeting.LiveVersionID).SetTriggerEvent(events.NameContactTagged).ExecX(ctx)

	_, err := tags.New(env.Bus).Apply(ctx, acme, fixtures.ContactLiamID, "onboarding-ready")
	require.NoError(t, err)
	enroller := &handedEnrollments{}
	require.NoError(t, events.AutomationsConsumer(enroller)(events.Message(onlyEnvelope(t, env, events.NameContactTagged))))
	require.Len(t, enroller.got, 1)
	handed := enroller.got[0]
	// The Enroller stores the Event before its trigger job runs, as jobs.Client does.
	require.NoError(t, events.Persist(ctx, env.DB, handed.Envelope))
	require.NoError(t, stepping.OnEvent(ctx, env.Bus, acme, handed.ContactID, automations.Event{
		Action: handed.Action, SourceID: handed.Envelope.SourceID(), OccurredAt: handed.Envelope.OccurredAt,
	}, handed.SourceAutomationID))

	run := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationJobsGreetingID), automationrun.ContactID(fixtures.ContactLiamID)).
		OnlyX(ctx)
	assert.Equal(t, events.NameContactTagged, *run.EventAction)
	assert.Equal(t, map[string]any{"tag": "onboarding-ready"}, run.EventProperties, "the steps see which Tag it was")
}

// A Rule triggered by automation.completed fires when an Enrollment completes, and what its Action
// then records (the Tag it applies) fires no Rule: Rules never chain (ADR 0054). Fixture: the
// Tag-clicked Rule, which applies rule-clicked, its Trigger turned to automation.completed.
func TestARuleTriggeredByACompletedEnrollmentFiresAndItsTagFiresNoRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	env.DB.RuleVersionTrigger.Update().
		Where(ruleversiontrigger.VersionID(fixtures.RuleTagClickedLiveVersionID)).
		SetAction(events.NameAutomationCompleted).
		ExecX(ctx)
	enrollVia(t, env, fixtures.AutomationJobsGreetingID, fixtures.ContactAliceID)
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	evaluator := &handedRuleEvents{}
	require.NoError(t, events.RulesConsumer(env.DB, evaluator)(events.Message(onlyEnvelope(t, env, events.NameAutomationCompleted))))
	require.Len(t, evaluator.got, 1)
	handed := evaluator.got[0]
	m := rules.New(env.Cipher, env.Bus)
	fired, err := m.Evaluate(ctx, acme, rules.Event{
		ContactID: handed.ContactID, Action: handed.Action, SourceID: handed.SourceID, RecordedAt: time.Now(), Properties: handed.Properties,
	})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	assert.EqualValues(t, fixtures.RuleTagClickedID, fired[0].RuleID)
	require.NoError(t, m.RunActions(ctx, acme, fired[0].ID, ruleRunners(env)))

	tagged := onlyEnvelope(t, env, events.NameContactTagged)
	require.NoError(t, events.RulesConsumer(env.DB, evaluator)(events.Message(tagged)))
	assert.Contains(t, tagsOf(t, env, fixtures.ContactAliceID), "rule-clicked")
	assert.NotEmpty(t, tagged.CausedBy, "the Action's Tag carries its Rule")
	assert.Len(t, evaluator.got, 1, "the Action's own contact.tagged is handed to no Rule")
}
