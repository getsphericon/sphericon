package mcpserver_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// handAlice publishes Alice's Event of the action, occurred now, and hands the stored Event to the
// Automations as its trigger job does.
func handAlice(t *testing.T, env *testhelper.TestEnv, action string, properties map[string]any) {
	t.Helper()
	ev := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: action, Properties: properties, OccurredAt: time.Now()}
	require.NoError(t, env.Publisher().Publish(t.Context(), ev))
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	last := stored[len(stored)-1]
	in := automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: last.Action, SourceID: *last.SourceID, OccurredAt: last.OccurredAt}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// An agent reads the Event a Wait for event arrived on, under its name, its action and properties
// wrapped as untrusted (ADR 0136): someone outside the Workspace wrote them.
func TestMCPEnrollmentsMarkTheKeptEventUntrusted(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	steps := automationstep.Root(
		automationstep.New(automationstep.WaitForEvent{
			Name: "purchase", TimeoutSeconds: 3600, Awaits: automationstep.AwaitedEvent{Matchers: []eventmatcher.Matcher{{Action: "order.created"}}},
			Arrived: automationstep.Root(), TimedOut: automationstep.Root(),
		}),
		automationstep.New(automationstep.Wait{Seconds: 3600}),
	)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Thanks for the purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "trial.started"}}, Steps: &steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	handAlice(t, env, "trial.started", nil)
	handAlice(t, env, "order.created", map[string]any{"note": injection})
	runID, err := env.DB.AutomationRun.Query().Where(automationrun.AutomationID(a.ID)).OnlyID(ctx)
	require.NoError(t, err)
	session := env.MCPClient(t, env.ScopedBearer(t, apitokens.ScopeEnrollmentsRead))

	res := call(t, session, "enrollments_get", map[string]any{"automationId": testhelper.ID(a.ID), "enrollmentId": testhelper.ID(runID)})

	require.False(t, res.IsError, text(t, res))
	enrollment := decode(t, text(t, res))
	assert.Equal(t, []any{map[string]any{
		"name":       "purchase",
		"action":     map[string]any{"untrusted_data": "order.created"},
		"properties": map[string]any{"untrusted_data": map[string]any{"note": injection}},
	}}, enrollment["awaited"])
}
