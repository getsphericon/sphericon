package events_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The goal Event is evaluated before the trigger's evaluation has run: its own evaluation finds no
// Enrollment to end, but its row was stored when it was published, so the trigger's evaluation
// then makes the Enrollment already at its goal (#751).
func TestAGoalEventConsumedBeforeTheTriggerEndsTheEnrollmentWhenItIsMade(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	failed := time.Now().Add(-time.Hour)
	paid := failed.Add(30 * time.Minute)
	publishAlone(t, env, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: paid,
	})

	require.NoError(t, env.StepExecution.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: "payment_succeeded", OccurredAt: paid}}))
	require.NoError(t, env.StepExecution.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: "payment_failed", OccurredAt: failed}}))

	run, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactAliceID)).
		Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
}
