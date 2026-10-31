package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
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
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	origin, err := automations.OriginOf(ctx, s, eventOf(t, env, fixtures.EventPaymentFailedID))
	require.NoError(t, err)

	runID, enrolled, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID, origin)
	require.NoError(t, err)
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)

	assert.True(t, enrolled)
	assert.Equal(t, new(fixtures.EventPaymentFailedAction), run.EventAction)
	assert.Equal(t, map[string]any{"amount": "€49", "plan": `<b>Pro</b> "annual"`}, run.EventProperties)
}

// An Event weighing more than a Rule is shown leaves no properties to copy, as a Rule sees none.
func TestAnEventOverTheBoundGivesNoPropertiesToCopy(t *testing.T) {
	env := testhelper.Setup(t)

	origin, err := automations.OriginOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), eventOf(t, env, fixtures.EventRuleUrlOversizedID))

	require.NoError(t, err)
	assert.Equal(t, fixtures.EventRuleUrlOversizedAction, origin.Action)
	assert.Nil(t, origin.Properties)
}

// A trigger job of a release before #752 names no stored Event: the Enrollment keeps the action and
// no properties.
func TestATriggerWithoutASourceIDGivesNoProperties(t *testing.T) {
	env := testhelper.Setup(t)
	ev := eventOf(t, env, fixtures.EventPaymentFailedID)
	ev.SourceID = ""

	origin, err := automations.OriginOf(context.Background(), env.DB.Scoped(fixtures.AcmeID), ev)

	require.NoError(t, err)
	assert.Equal(t, fixtures.EventPaymentFailedAction, origin.Action)
	assert.Nil(t, origin.Properties)
}

// An Enrollment no Event began keeps no copy: its `event` is empty.
func TestAnEnrollmentNoEventBeganKeepsNoCopy(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	runID, _, err := automations.Enroll(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.NoError(t, err)
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)

	assert.Nil(t, run.EventAction)
	assert.Nil(t, run.EventProperties)
}
