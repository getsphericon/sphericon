package automations_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Which `sms.` Events may start, end or complete an Automation (#1032): what the person does, not
// what the send infrastructure records. And the SMS step's checks (ADR 0081): what a save and
// Publish refuse.

func TestSmsReceivedTriggersExitsAndCompletesButSmsSentDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
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

// A delivery report of a text is the person's state on the Channel, not the send's record: unlike
// email's, sms.delivered and sms.undelivered may start, end and complete an Automation (#1032).
func TestSmsDeliveredAndUndeliveredTriggerExitAndComplete(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	delivered, undelivered := "sms.delivered", "sms.undelivered"

	onUndelivered, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Undelivered", TriggerEvent: undelivered, ExitEvents: []string{delivered},
	})
	require.NoError(t, err)
	assert.NotZero(t, onUndelivered.ID)
	onDelivered, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Delivered", TriggerEvent: delivered, ExitEvents: []string{}, GoalEvent: &undelivered,
	})
	require.NoError(t, err)
	assert.NotZero(t, onDelivered.ID)
}

// A new SMS opt-out of a Contact's number (sms.unsubscribed) may start and end an Automation, as
// telegram.unsubscribed and email.unsubscribed may.
func TestSmsUnsubscribedTriggersAndExits(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	unsubscribed, confirmed := "sms.unsubscribed", "sms.confirmed"

	onStop, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Unsubscribed", TriggerEvent: unsubscribed, ExitEvents: []string{confirmed},
	})
	require.NoError(t, err)
	assert.NotZero(t, onStop.ID)
	onConsent, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Confirmed", TriggerEvent: confirmed, ExitEvents: []string{unsubscribed},
	})
	require.NoError(t, err)
	assert.NotZero(t, onConsent.ID)
}

func smsVia(id *int64, text string) automationstep.Step {
	return automationstep.Step{Type: automationstep.SMS, IntegrationID: id, Text: text}
}

func TestPublishIsRefusedForAnSMSStepThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	email := int64(fixtures.IntegrationAcmeDefaultID)
	cases := map[string]*int64{
		"no default SMS Integration":        nil,
		"an Integration of another Channel": &email,
	}
	for name, integration := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
				Steps: &[]automationstep.Step{smsVia(integration, "Hi")},
			})
			require.NoError(t, err, "a draft may name it")

			_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID)

			require.ErrorIs(t, err, outboundmessages.ErrRouteHeld)
		})
	}
}

func TestPublishSendsAnSMSStepThroughItsTwilioIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	id := int64(env.AcmeTwilio(t).Integration.ID)
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationstep.Step{smsVia(&id, "Hi")},
	})
	require.NoError(t, err)

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID)

	require.NoError(t, err)
}

func TestAnSMSStepNeedsATextThatRenders(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	for name, text := range map[string]string{"blank": "  ", "bad Liquid": "{% if %}"} {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
				Steps: &[]automationstep.Step{smsVia(nil, text)},
			})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}

// Twilio sends up to 700 UCS-2 characters (eleven segments and more): 700 save, one more is refused.
func TestSavingAnSMSStepOverTwiliosLengthLimitIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	atLimit := strings.Repeat("я", 700)

	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationstep.Step{smsVia(nil, atLimit)},
	})
	require.NoError(t, err)
	_, err = m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationstep.Step{smsVia(nil, atLimit+"я")},
	})

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.ErrorContains(t, err, "step 1: the text is 701 characters in UCS-2 (11 segments); it may take 700 characters in UCS-2 at most")
}
