package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// eventOf is a stored fixture Event as its trigger job hands it on.
func eventOf(t *testing.T, env *testhelper.TestEnv, id int64) automations.Event {
	t.Helper()
	stored, err := env.DB.Event.Get(context.Background(), id)
	require.NoError(t, err)
	return automations.Event{Action: stored.Action, SourceID: *stored.SourceID, OccurredAt: stored.OccurredAt}
}

// The Enrollment an Event starts keeps that Event's action and properties (#752), copied from the
// stored row when it begins.
func TestATriggeredEnrollmentCopiesItsEventsActionAndProperties(t *testing.T) {
	env := testhelper.Setup(t)
	ev := eventOf(t, env, fixtures.EventPaymentFailedID)
	in := automations.OnEventInput{ContactID: fixtures.ContactPaymentFailedID, Event: ev}

	require.NoError(t, env.StepExecution.OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), in))

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

	_, err := env.StepExecution.EnrollOnRule(context.Background(), env.DB.Scoped(fixtures.AcmeID), in)

	require.NoError(t, err)
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.Len(t, runs, 1)
	assert.Equal(t, new(fixtures.EventRuleUrlOversizedAction), runs[0].EventAction)
	assert.Nil(t, runs[0].EventProperties)
}

// A trigger job of a release before #752 names no stored Event: the Enrollment keeps the action and
// no properties.
func TestATriggerWithoutASourceIDGivesNoProperties(t *testing.T) {
	env := testhelper.Setup(t)
	ev := eventOf(t, env, fixtures.EventPaymentFailedID)
	ev.SourceID = ""
	in := automations.OnEventInput{ContactID: fixtures.ContactPaymentFailedID, Event: ev}

	require.NoError(t, env.StepExecution.OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), in))

	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.Len(t, runs, 1)
	assert.Equal(t, new(fixtures.EventPaymentFailedAction), runs[0].EventAction)
	assert.Nil(t, runs[0].EventProperties)
}

// An Enrollment no Event began (a Rule's whose Event is no longer stored) keeps no copy: its
// `event` is empty.
func TestAnEnrollmentNoEventBeganKeepsNoCopy(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := enrollNow(t, env, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.NoError(t, err)
	run, err := env.DB.AutomationRun.Get(ctx, runOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID))
	require.NoError(t, err)

	assert.Nil(t, run.EventAction)
	assert.Nil(t, run.EventProperties)
}
