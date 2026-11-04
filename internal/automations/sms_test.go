package automations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Which `sms.` Events may start, end or complete an Automation (#1032): what the person does, not
// what the send infrastructure records.

func TestSmsReceivedTriggersExitsAndCompletesButSmsSentDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	received, confirmed, sent := "sms.received", "sms.confirmed", "sms.sent"

	onReply, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Reply", TriggerEvent: received, ExitEvents: []string{"contact.tagged"}, GoalEvent: &confirmed,
	})
	require.NoError(t, err)
	assert.NotZero(t, onReply.ID)
	ending, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Ends on a reply", TriggerEvent: confirmed, ExitEvents: []string{received},
	})
	require.NoError(t, err)
	assert.NotZero(t, ending.ID)

	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Sent", TriggerEvent: sent, ExitEvents: []string{}})
	require.ErrorIs(t, err, automations.ErrInvalidTrigger)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Exit", TriggerEvent: received, ExitEvents: []string{sent}})
	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Goal", TriggerEvent: received, ExitEvents: []string{}, GoalEvent: &sent})
	require.ErrorIs(t, err, automations.ErrInvalidGoalEvent)
}
