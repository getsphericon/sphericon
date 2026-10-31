package jobs_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// storedEventOf is a stored fixture Event as its trigger job hands it on (#752).
func storedEventOf(t *testing.T, env *testhelper.TestEnv, id int64) automations.Event {
	t.Helper()
	stored, err := env.DB.Event.Get(context.Background(), id)
	require.NoError(t, err)
	return automations.Event{Action: stored.Action, SourceID: *stored.SourceID, OccurredAt: *stored.OccurredAt}
}

// An Automation's email renders the properties of the Event that started the Enrollment (#752):
// "your payment of €49 failed". A value is HTML-escaped as a Contact's is, and a property the Event
// lacks (card_hint) renders empty.
func TestAnAutomationEmailRendersTheTriggeringEventsProperties(t *testing.T) {
	env := testhelper.Setup(t)
	ev := storedEventOf(t, env, fixtures.EventPaymentFailedID)

	require.NoError(t, stepping.OnEvent(t.Context(), env.Bus, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactPaymentFailedID, ev, 0))
	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	require.Len(t, fs.sent, 2)
	assert.Contains(t, fs.sent[0].Text, `Your payment of €49 for <b>Pro</b> "annual" did not go through. Please update your card.`)
	assert.NotContains(t, fs.sent[0].HTML, "<b>Pro</b>", "the value is escaped, not markup")
}

// An Enrollment no Event began renders every `event` tag empty.
func TestAnAutomationEmailOfAnEnrollmentNoEventBeganRendersTheEventEmpty(t *testing.T) {
	env := testhelper.Setup(t)
	runID, enrolled, err := automations.Enroll(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaymentFailedID)
	require.NoError(t, err)
	require.True(t, enrolled)
	queueStep(t, env, runID)

	fs := &fakeSender{}
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(fs)))

	require.Len(t, fs.sent, 2)
	assert.Contains(t, fs.sent[0].Text, "Your payment of for did not go through. Please update your card.", "the text part collapses the spaces")
}
