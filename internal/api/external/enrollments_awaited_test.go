package external_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// recordFor publishes Alice's Event of the action, occurred now, which stores its row, and hands
// the stored Event to the Automations as its trigger job does.
func recordFor(t *testing.T, env *testhelper.TestEnv, action string, properties map[string]any) {
	t.Helper()
	ev := &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: action, Properties: properties, OccurredAt: time.Now()}
	require.NoError(t, env.Publisher().Publish(t.Context(), ev))
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	last := stored[len(stored)-1]
	in := automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: last.Action, SourceID: *last.SourceID, OccurredAt: last.OccurredAt}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// waitingForAPurchase publishes an Automation on trial.started that waits an hour for an order
// under the name `purchase`, then waits an hour, ended by subscription.cancelled, enrolls Alice and
// returns the ids of the Automation and of her Enrollment, standing on the Wait for event.
func waitingForAPurchase(t *testing.T, env *testhelper.TestEnv) (automationID, runID int64) {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	steps := automationstep.Root(
		automationstep.New(automationstep.WaitForEvent{
			Name: "purchase", TimeoutSeconds: 3600, Awaits: automationstep.AwaitedEvent{Actions: []string{"order.created"}},
			Arrived: automationstep.Root(), TimedOut: automationstep.Root(),
		}),
		automationstep.New(automationstep.Wait{Seconds: 3600}),
	)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Thanks for the purchase", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "trial.started"}},
		ExitEvents: []eventmatcher.Matcher{{Action: "subscription.cancelled"}}, Steps: &steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	recordFor(t, env, "trial.started", nil)
	runID, err = env.DB.AutomationRun.Query().Where(automationrun.AutomationID(a.ID)).OnlyID(ctx)
	require.NoError(t, err)
	return a.ID, runID
}

// An Enrollment returns the Event its Wait for event arrived on by the Step's name while it runs,
// in the list and alone, and no longer once it ends.
func TestExternalEnrollmentsReturnTheKeptEventWhileTheEnrollmentRuns(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)
	automationID, runID := waitingForAPurchase(t, env)
	get := externalapi.EnrollmentsGetParams{AutomationId: entityIDString(automationID), EnrollmentId: entityIDString(runID)}

	waiting, err := c.EnrollmentsGet(ctx, get)
	require.NoError(t, err)
	recordFor(t, env, "order.created", map[string]any{"product": "Lamp"})
	running, err := c.EnrollmentsGet(ctx, get)
	require.NoError(t, err)
	listed, err := c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{AutomationId: entityIDString(automationID)})
	require.NoError(t, err)
	recordFor(t, env, "subscription.cancelled", nil)
	ended, err := c.EnrollmentsGet(ctx, get)
	require.NoError(t, err)

	assert.Empty(t, waiting.Awaited, "nothing arrived yet")
	require.Len(t, running.Awaited, 1)
	kept := running.Awaited[0]
	assert.Equal(t, "purchase", kept.Name)
	assert.Equal(t, "order.created", kept.Action)
	assert.JSONEq(t, `"Lamp"`, string(kept.Properties["product"]))
	require.Len(t, listed.Items, 1)
	assert.Equal(t, running.Awaited, listed.Items[0].Awaited, "the list returns it too")
	assert.Equal(t, externalapi.EnrollmentStatusExited, ended.Status)
	assert.Empty(t, ended.Awaited, "cleared when it ended")
}
