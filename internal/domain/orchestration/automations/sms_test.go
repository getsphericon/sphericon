package automations_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Which `sms.` Events may start, end or complete an Automation (#1032): what the person does, not
// what the send infrastructure records. And the SMS step's checks (ADR 0081): what a save and
// Publish refuse.

func TestSmsReceivedTriggersExitsAndCompletesButSmsSentDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	received, confirmed, sent := systemevent.SmsReceived, systemevent.SmsConfirmed, systemevent.SmsSent

	onReply, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Reply", Trigger: eventmatcher.Matcher{Action: received}, ExitEvents: []eventmatcher.Matcher{{Action: systemevent.ContactTagged}}, GoalEvent: &eventmatcher.Matcher{Action: confirmed},
	})
	require.NoError(t, err)
	assert.NotZero(t, onReply.ID)
	ending, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Ends on a reply", Trigger: eventmatcher.Matcher{Action: confirmed}, ExitEvents: []eventmatcher.Matcher{{Action: received}},
	})
	require.NoError(t, err)
	assert.NotZero(t, ending.ID)

	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Sent", Trigger: eventmatcher.Matcher{Action: sent}, ExitEvents: []eventmatcher.Matcher{}})
	require.ErrorIs(t, err, automations.ErrInvalidTrigger)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Exit", Trigger: eventmatcher.Matcher{Action: received}, ExitEvents: []eventmatcher.Matcher{{Action: sent}}})
	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Goal", Trigger: eventmatcher.Matcher{Action: received}, ExitEvents: []eventmatcher.Matcher{}, GoalEvent: &eventmatcher.Matcher{Action: sent}})
	require.ErrorIs(t, err, automations.ErrInvalidGoalEvent)
}

// A delivery report of a text is the person's state on the Channel, not the send's record: unlike
// email's, sms.delivered and sms.undelivered may start, end and complete an Automation (#1032).
func TestSmsDeliveredAndUndeliveredTriggerExitAndComplete(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	delivered, undelivered := systemevent.SmsDelivered, systemevent.SmsUndelivered

	onUndelivered, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Undelivered", Trigger: eventmatcher.Matcher{Action: undelivered}, ExitEvents: []eventmatcher.Matcher{{Action: delivered}},
	})
	require.NoError(t, err)
	assert.NotZero(t, onUndelivered.ID)
	onDelivered, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Delivered", Trigger: eventmatcher.Matcher{Action: delivered}, ExitEvents: []eventmatcher.Matcher{}, GoalEvent: &eventmatcher.Matcher{Action: undelivered},
	})
	require.NoError(t, err)
	assert.NotZero(t, onDelivered.ID)
}

// A new SMS opt-out of a Contact's number (sms.unsubscribed) may start and end an Automation, as
// telegram.unsubscribed and email.unsubscribed may.
func TestSmsUnsubscribedTriggersAndExits(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	unsubscribed, confirmed := systemevent.SmsUnsubscribed, systemevent.SmsConfirmed

	onStop, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Unsubscribed", Trigger: eventmatcher.Matcher{Action: unsubscribed}, ExitEvents: []eventmatcher.Matcher{{Action: confirmed}},
	})
	require.NoError(t, err)
	assert.NotZero(t, onStop.ID)
	onConsent, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Confirmed", Trigger: eventmatcher.Matcher{Action: confirmed}, ExitEvents: []eventmatcher.Matcher{{Action: unsubscribed}},
	})
	require.NoError(t, err)
	assert.NotZero(t, onConsent.ID)
}

func smsVia(id *int64, text string) automationstep.SMS {
	return automationstep.SMS{IntegrationID: id, Text: text}
}

func TestPublishIsRefusedForAnSMSStepThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	email := int64(fixtures.IntegrationAcmeDefaultID)
	cases := map[string]*int64{
		"no default SMS Integration":        nil,
		"an Integration of another Channel": &email,
	}
	for name, integration := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
				Steps: steps(smsVia(integration, "Hi")),
			})
			require.NoError(t, err, "a draft may name it")

			_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

			require.ErrorIs(t, err, outboundmessages.ErrRouteHeld)
		})
	}
}

func TestPublishSendsAnSMSStepThroughItsTwilioIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	id := int64(env.AcmeTwilio(t).Integration.ID)
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
		Steps: steps(smsVia(&id, "Hi")),
	})
	require.NoError(t, err)

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

	require.NoError(t, err)
}

func TestAnSMSStepNeedsATextThatRenders(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	for name, text := range map[string]string{"blank": "  ", "bad Liquid": "{% if %}"} {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
				Steps: steps(smsVia(nil, text)),
			})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}

// Twilio sends up to 700 UCS-2 characters (eleven segments and more): 700 save, one more is refused.
func TestSavingAnSMSStepOverTwiliosLengthLimitIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	atLimit := strings.Repeat("я", 700)

	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
		Steps: steps(smsVia(nil, atLimit)),
	})
	require.NoError(t, err)
	over := automationstep.Root(automationstep.Step{ID: "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b01", Payload: smsVia(nil, atLimit+"я")})
	_, err = m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
		Steps: &over,
	})

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.ErrorContains(t, err, "step 5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b01 (sms): the text is 701 characters in UCS-2 (11 segments); it may take 700 characters in UCS-2 at most")
}
