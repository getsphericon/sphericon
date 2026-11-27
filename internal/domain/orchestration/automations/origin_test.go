package automations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// eventOf is a stored fixture Event as its trigger job hands it on.
func eventOf(t *testing.T, env *testhelper.TestEnv, id int64) automations.Event {
	t.Helper()
	stored, err := env.DB.Event.Get(t.Context(), id)
	require.NoError(t, err)
	return automations.Event{Action: stored.Action, SourceID: *stored.SourceID, OccurredAt: stored.OccurredAt}
}

// The Enrollment an Event starts keeps that Event's action and properties (#752), copied from the
// stored row when it begins.
func TestATriggeredEnrollmentCopiesItsEventsActionAndProperties(t *testing.T) {
	env := testhelper.Setup(t)
	ev := eventOf(t, env, fixtures.EventPaymentFailedID)
	in := automations.OnEventInput{ContactID: fixtures.ContactPaymentFailedID, Event: ev}

	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))

	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.Len(t, runs, 1)
	assert.Equal(t, new(fixtures.EventPaymentFailedAction), runs[0].EventAction)
	assert.Equal(t, map[string]any{"amount": "€49", "plan": `<b>Pro</b> "annual"`}, runs[0].EventProperties)
}

// An Event weighing more than a Rule is shown leaves no properties to copy, as a Rule sees none.
func TestAnEventOverTheBoundGivesNoPropertiesToCopy(t *testing.T) {
	env := testhelper.Setup(t)
	in := automations.EnrollOnRuleInput{
		AutomationID: fixtures.AutomationPaymentRecoveryID, ContactID: fixtures.ContactPaymentFailedID,
		SourceID: eventOf(t, env, fixtures.EventRuleUrlOversizedID).SourceID,
	}

	_, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(t.Context(), env.DB.Scoped(fixtures.AcmeID), in)

	require.NoError(t, err)
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.Len(t, runs, 1)
	assert.Equal(t, new(fixtures.EventRuleUrlOversizedAction), runs[0].EventAction)
	assert.Nil(t, runs[0].EventProperties)
}

// An Event that does not say when it occurred is refused: the Enrollment it would make would not
// know when it began.
func TestAnEventWithoutItsTimeIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ev := eventOf(t, env, fixtures.EventPaymentFailedID)
	ev.OccurredAt = time.Time{}
	in := automations.OnEventInput{ContactID: fixtures.ContactPaymentFailedID, Event: ev}

	err := testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in)

	require.Error(t, err)
	assert.Empty(t, runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID))
}
