package automations_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Telegram step's checks (ADR 0081): what Publish refuses and which `telegram.` Events may
// start, end or complete an Automation.

func telegramVia(id *int64, text string) automationstep.Step {
	return automationstep.Step{Type: automationstep.Telegram, IntegrationID: id, Text: text}
}

func TestPublishIsRefusedForATelegramStepThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	email := int64(fixtures.IntegrationAcmeDefaultID)
	cases := map[string]*int64{
		"no default Telegram Integration":   nil,
		"an Integration of another Channel": &email,
	}
	for name, integration := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
				Steps: &[]automationstep.Step{telegramVia(integration, "Hi")},
			})
			require.NoError(t, err, "a draft may name it")

			_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID)

			require.ErrorIs(t, err, outboundmessages.ErrRouteHeld)
		})
	}
}

func TestPublishIsRefusedForATextOverTheLimitAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationstep.Step{telegramVia(nil, strings.Repeat("a", 4097))},
	})
	require.NoError(t, err, "saving warns, it does not refuse")

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID)

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.Contains(t, err.Error(), "4097")
}

func TestATelegramStepNeedsATextThatRenders(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	for name, text := range map[string]string{"blank": "  ", "bad Liquid": "{% if %}"} {
		t.Run(name, func(t *testing.T) {
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
				Steps: &[]automationstep.Step{telegramVia(nil, text)},
			})

			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}

func TestTelegramBoundTriggersExitsAndCompletesButTelegramSentDoesNot(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	goal := "telegram.unbound"

	created, err := m.Create(t.Context(), s, automations.CreateInput{
		Name: "Bound", TriggerEvent: "telegram.bound", ExitEvents: []string{"telegram.unsubscribed"}, GoalEvent: &goal,
	})
	require.NoError(t, err)
	assert.NotZero(t, created.ID)

	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Sent", TriggerEvent: "telegram.sent", ExitEvents: []string{}})
	require.ErrorIs(t, err, automations.ErrInvalidTrigger)
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Exit", TriggerEvent: "telegram.bound", ExitEvents: []string{"telegram.sent"}})
	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
	sent := "telegram.sent"
	_, err = m.Create(t.Context(), s, automations.CreateInput{Name: "Goal", TriggerEvent: "telegram.bound", ExitEvents: []string{}, GoalEvent: &sent})
	require.ErrorIs(t, err, automations.ErrInvalidGoalEvent)
}

func TestPublishIsRefusedForACaptionOverTheLimitAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	step := telegramVia(nil, strings.Repeat("a", 1025))
	step.ImageURL = "https://cdn.example.com/a.png"
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &[]automationstep.Step{step}})
	require.NoError(t, err, "saving warns, it does not refuse")

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID)

	require.ErrorIs(t, err, automations.ErrInvalidStep)
	assert.Contains(t, err.Error(), "1025")
}

func TestATelegramStepRefusesABadImageAndBadButtons(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	cases := map[string]func(*automationstep.Step){
		"an http image": func(st *automationstep.Step) { st.ImageURL = "http://cdn.example.com/a.png" },
		"a blank label": func(st *automationstep.Step) {
			st.Buttons = []telegramrender.Button{{Label: "", URL: "https://x.test"}}
		},
		"nine buttons":    func(st *automationstep.Step) { st.Buttons = make([]telegramrender.Button, 9) },
		"a relative link": func(st *automationstep.Step) { st.Buttons = []telegramrender.Button{{Label: "a", URL: "/x"}} },
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			step := telegramVia(nil, "Hi")
			edit(&step)
			_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &[]automationstep.Step{step}})
			require.ErrorIs(t, err, automations.ErrInvalidStep)
		})
	}
}
