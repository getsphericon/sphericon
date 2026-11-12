package jobs_test

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Telegram send step (ADR 0081): a bound Contact gets the rendered text through the bot the
// step names, a Contact with no Binding is skipped and the sequence goes on. The bot is the fake
// Bot API; the step runs through the Outbound module of the harness, which builds it as production.

// telegramAutomation creates and publishes an Automation triggered on telegram.bound that runs
// steps, and returns its id.
func telegramAutomation(t *testing.T, env *testhelper.TestEnv, steps ...automationstep.Step) int64 {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	created, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Telegram welcome", TriggerEvent: systemevent.TelegramBound, ExitEvents: []string{}, Steps: steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, created.ID, testhelper.AutomationRevisionIn(ctx, t, s, created.ID))
	require.NoError(t, err)
	return created.ID
}

// telegramStep is a Telegram step through the bot.
func telegramStep(bot testhelper.TelegramBot, text string) automationstep.Step {
	id := int64(bot.Integration.ID)
	return automationstep.Step{Type: automationstep.Telegram, IntegrationID: &id, Text: text}
}

// runSteps drives the queued steps through the harness's own Outbound module.
func runSteps(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	env.RunQueuedSteps(t, stepperAt(env, env.OutboundModule(), passingClock()))
}

func TestATelegramStepSendsTheRenderedAndEscapedTextToTheBoundChat(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetFirstName("<i>Bob</i> & co").ExecX(t.Context())
	automationID := int64(fixtures.AutomationJobsTelegramEscapedID)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555001, 555001)
	runID := int64(fixtures.AutomationRunJobsTelegramEscapedBobID)
	queueStep(t, env, runID)

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
	assert.Equal(t, channels.Telegram, msg.Channel)
	assert.Equal(t, "555001", msg.Destination)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, map[string]any{
		"contactId": fixtures.ContactBobID, "automationId": automationID, "destination": "555001", "integrationId": bot.Integration.ID,
	}))
}

func TestAContactWithTwoBindingsGetsTwoCallsAndOneOutboundMessage(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555011, 555011)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555012, 555012)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)

	runSteps(t, env)

	chats := lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
	assert.Equal(t, []string{"555011", "555012"}, chats)
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).AllX(t.Context())
	require.Len(t, msgs, 1)
	assert.Equal(t, "555011", msgs[0].Destination, "the first Binding that accepted it")
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, nil))
}

func TestAMixedSequenceSendsTheEmailsAndRecordsTheTelegramSkipForAContactWithNoBinding(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTelegram(t)
	runID := int64(fixtures.AutomationRunJobsTelegramBetweenEmailsBobID)
	queueStep(t, env, runID)

	runSteps(t, env)

	subjects := lo.Map(env.CustomerMail.Messages(), func(m messaging.EmailMessage, _ int) string { return m.Subject })
	assert.Equal(t, []string{"First", "Last"}, subjects)
	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(t.Context(), runID).Status)
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(t.Context())
	assert.Equal(t, "no_destination", *skipped.Reason)
	assert.Equal(t, 1, *skipped.AutomationStep)
	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramSent, nil))
}

func TestARenderedTextOverTheLimitFailsThatMessageAlone(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
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

func TestATelegramStepWithAnImageSendsAPhotoWithTheCaptionAndButtons(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555031, 555031)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramPhotoBobID)

	runSteps(t, env)

	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	photos := env.Telegram.RequestsOf(telegramtest.SendPhoto)
	require.Len(t, photos, 1)
	assert.Equal(t, "https://cdn.example.com/a.png", photos[0].String("photo"))
	assert.Equal(t, "Look <b>here</b>", photos[0].String("caption"))
	assert.Equal(t, "HTML", photos[0].String("parse_mode"))
	assert.Regexp(t,
		`^\{"inline_keyboard":\[\[\{"text":"Open","url":"https?://[^"/]+/c/[A-Za-z0-9_-]+"\}\],\[\{"text":"More","url":"https?://[^"/]+/c/[A-Za-z0-9_-]+"\}\]\]\}$`,
		photos[0].String("reply_markup"), "the link buttons are short links (ADR 0083)")
}

func TestTurningTheLinkPreviewOffSetsTheOption(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555041, 555041)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramPreviewOffBobID)

	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 2)
	assert.JSONEq(t, `{"is_disabled":true}`, sends[0].String("link_preview_options"))
	assert.Empty(t, sends[1].String("link_preview_options"), "on by default")
}

func TestARenderedCaptionOverTheCaptionLimitFails(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetFirstName(strings.Repeat("x", 100)).ExecX(t.Context())
	step := telegramStep(bot, strings.Repeat("a", 950)+"{{ first_name }}")
	step.ImageURL = "https://cdn.example.com/a.png"
	automationID := telegramAutomation(t, env, step)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555051, 555051)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	err := stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID)

	require.ErrorContains(t, err, "content_too_long")
	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendPhoto))
}

func TestTheUnsubscribeLinkOptsTheChatOutOfTheAutomationOnTelegramOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	emailOptOuts := func() int {
		return env.DB.Unsubscribe.Query().Where(unsubscribe.ChannelEQ(channels.Email), unsubscribe.ContactID(fixtures.ContactBobID)).CountX(ctx)
	}
	emailBefore := emailOptOuts()
	automationID := int64(fixtures.AutomationJobsTelegramUnsubscribeLinkID)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555061, 555061)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramUnsubscribeLinkBobID)
	runSteps(t, env)
	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	link := regexp.MustCompile(`/e/u/([^"]+)"`).FindStringSubmatch(sends[0].String("text"))
	require.Len(t, link, 2, "the text carries the unsubscribe link")

	err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})

	require.NoError(t, err)
	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ("555061")).OnlyX(ctx)
	assert.Equal(t, channels.Telegram, optOut.Channel)
	assert.Equal(t, eligibility.AutomationSource(automationID), optOut.SendingSource)
	assert.Equal(t, emailBefore, emailOptOuts(), "email to the same Contact keeps sending")
	recorded := env.QueuedMessages(t, systemevent.TelegramUnsubscribed)
	require.Len(t, recorded, 1, "the link records telegram.unsubscribed")
	assert.Equal(t, strconv.Itoa(fixtures.ContactBobID), fmt.Sprint(recorded[0].Data["contactId"]))
	assert.Equal(t, events.UnsubscribedByLink, recorded[0].Data["via"])
	assert.Equal(t, strconv.FormatInt(automationID, 10), fmt.Sprint(recorded[0].Data["automationId"]))
	assert.Zero(t, env.QueuedCount(t, systemevent.EmailUnsubscribed, nil), "and no email event")

	err = env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})

	require.NoError(t, err)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramUnsubscribed, nil), "a second click records nothing")
}
