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
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Telegram step's checks (ADR 0081): what Publish refuses and which `telegram.` Events may
// start, end or complete an Automation.

func telegramVia(id *int64, text string) automationstep.Telegram {
	return automationstep.Telegram{IntegrationID: id, Text: text}
}

func TestPublishIsRefusedForATelegramStepThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	email := int64(fixtures.IntegrationAcmeDefaultID)
	cases := map[string]*int64{
		"no default Telegram Integration":   nil,
		"an Integration of another Channel": &email,
	}
	for name, integration := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
				Steps: steps(telegramVia(integration, "Hi")),
			})
			require.NoError(t, err, "a draft may name it")

			_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

			require.ErrorIs(t, err, outboundmessages.ErrRouteHeld)
		})
	}
}

func TestPublishIsRefusedForATextOverTheLimitAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
		Steps: steps(telegramVia(nil, strings.Repeat("a", 4097))),
	})
	require.NoError(t, err, "saving warns, it does not refuse")

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.Contains(t, err.Error(), "4097")
}

func TestATelegramStepNeedsATextThatRenders(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	for name, text := range map[string]string{"blank": "  ", "bad Liquid": "{% if %}"} {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
				Steps: steps(telegramVia(nil, text)),
			})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}

func TestTelegramBoundTriggersExitsAndCompletesButTelegramSentDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	goal := systemevent.TelegramUnbound

	created, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Bound", TriggerEvent: systemevent.TelegramBound, ExitEvents: []string{systemevent.TelegramUnsubscribed}, GoalEvent: &goal,
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)

	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Sent", TriggerEvent: systemevent.TelegramSent, ExitEvents: []string{}})
	require.ErrorIs(t, err, automations.ErrInvalidTrigger)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Exit", TriggerEvent: systemevent.TelegramBound, ExitEvents: []string{systemevent.TelegramSent}})
	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
	sent := systemevent.TelegramSent
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Goal", TriggerEvent: systemevent.TelegramBound, ExitEvents: []string{}, GoalEvent: &sent})
	require.ErrorIs(t, err, automations.ErrInvalidGoalEvent)
}

func TestPublishIsRefusedForACaptionOverTheLimitAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	step := telegramVia(nil, strings.Repeat("a", 1025))
	step.ImageURL = "https://cdn.example.com/a.png"
	caption := automationstep.Root(automationstep.Step{ID: "5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b03", Payload: step})
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID), Steps: &caption})
	require.NoError(t, err, "saving warns, it does not refuse")

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.Contains(t, err.Error(), "1025")
	assert.Contains(t, err.Error(), "step 5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b03 (telegram)", "the refusal names the Step by its id and kind")
}

func TestATelegramStepRefusesABadImageAndBadButtons(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	cases := map[string]func(*automationstep.Telegram){
		"an http image": func(st *automationstep.Telegram) { st.ImageURL = "http://cdn.example.com/a.png" },
		"a blank label": func(st *automationstep.Telegram) {
			st.Buttons = []telegramrender.Button{{Label: "", URL: "https://x.test"}}
		},
		"nine buttons":    func(st *automationstep.Telegram) { st.Buttons = make([]telegramrender.Button, 9) },
		"a relative link": func(st *automationstep.Telegram) { st.Buttons = []telegramrender.Button{{Label: "a", URL: "/x"}} },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			step := telegramVia(nil, "Hi")
			edit(&step)
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID), Steps: steps(step)})
			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}
