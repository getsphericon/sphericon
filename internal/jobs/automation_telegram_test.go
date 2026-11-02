package jobs_test

import (
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Telegram send step (ADR 0081): a bound Contact gets the rendered text through the bot the
// step names, a Contact with no Binding is skipped and the sequence goes on. The bot is the fake
// Bot API; the step runs through the Outbound module of the harness, which builds it as production.

// telegramAutomation creates and publishes an Automation triggered on telegram.bound that runs
// steps, and returns its id.
func telegramAutomation(t *testing.T, env *testhelper.TestEnv, steps ...automationsteps.Step) int64 {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Telegram welcome", TriggerEvent: events.NameTelegramBound, ExitEvents: []string{}, Steps: steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID)
	require.NoError(t, err)
	return created.ID
}

// telegramStep is a Telegram step through the bot.
func telegramStep(bot testhelper.TelegramBot, text string) automationsteps.Step {
	id := int64(bot.Integration.ID)
	return automationsteps.Step{Type: automationsteps.Telegram, IntegrationID: &id, Text: text}
}

// runSteps drives the queued steps through the harness's own Outbound module.
func runSteps(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.RunQueuedSteps(t, stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock())))
}

func TestATelegramStepSendsTheRenderedAndEscapedTextToTheBoundChat(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7301, "acme_step_bot")
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetFirstName("<i>Bob</i> & co").ExecX(t.Context())
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hi {{ first_name }}, <b>welcome</b><script>x()</script>"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555001, 555001)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	assert.Equal(t, bot.Token, sends[0].Token)
	assert.Equal(t, "555001", sends[0].String("chat_id"))
	assert.Equal(t, "HTML", sends[0].String("parse_mode"))
	assert.Equal(t, "Hi &lt;i&gt;Bob&lt;/i&gt; &amp; co, <b>welcome</b>", sends[0].String("text"))
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, outboundmessage.ChannelTelegram, msg.Channel)
	assert.Equal(t, "555001", msg.Destination)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, map[string]any{
		"contactId": fixtures.ContactBobID, "automationId": automationID, "destination": "555001", "integrationId": bot.Integration.ID,
	}))
}

func TestAContactWithTwoBindingsGetsTwoCallsAndOneOutboundMessage(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7302, "acme_two_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555011, 555011)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555012, 555012)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	chats := lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
	assert.Equal(t, []string{"555011", "555012"}, chats)
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).AllX(t.Context())
	require.Len(t, msgs, 1)
	assert.Equal(t, "555011", msgs[0].Destination, "the first Binding that accepted it")
	assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, nil))
}

func TestAMixedSequenceSendsTheEmailsAndRecordsTheTelegramSkipForAContactWithNoBinding(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7303, "acme_mixed_bot")
	automationID := telegramAutomation(t, env,
		automationsteps.Step{Type: automationsteps.Email, Subject: "First", Body: "<p>one</p>"},
		telegramStep(bot, "Telegram"),
		automationsteps.Step{Type: automationsteps.Email, Subject: "Last", Body: "<p>two</p>"},
	)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	subjects := lo.Map(env.CustomerMail.Messages(), func(m messaging.EmailMessage, _ int) string { return m.Subject })
	assert.Equal(t, []string{"First", "Last"}, subjects)
	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(t.Context())
	assert.Equal(t, "no_destination", *skipped.Reason)
	assert.Equal(t, 1, *skipped.AutomationStep)
	assert.Zero(t, env.OutboxCount(t, events.NameTelegramSent, nil))
}

func TestARenderedTextOverTheLimitFailsThatMessageAlone(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7304, "acme_long_bot")
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetFirstName(strings.Repeat("x", 300)).ExecX(t.Context())
	automationID := telegramAutomation(t, env, telegramStep(bot, strings.Repeat("a", 4000)+"{{ first_name }}"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555021, 555021)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	err := stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID)

	require.ErrorContains(t, err, "content_too_long", "the Enrollment fails with the message's reason")
	assert.Equal(t, automationrun.StatusFailed, env.DB.AutomationRun.GetX(t.Context(), runID).Status)

	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Equal(t, "content_too_long", *msg.Reason)
}
