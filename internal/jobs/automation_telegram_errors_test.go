package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// What Telegram answers at send time is sorted by whom it concerns (ADR 0079): the bot, the rate,
// the message or the chat. The bot is the fake Bot API, scripted to refuse.

// A 401 holds the bot: the Enrollment waits at its step with the Hold, nothing is recorded as sent,
// and saving the Integration with a token Telegram accepts lets the same step send.
func TestATelegramUnauthorizedHoldsTheBotUntilItsTokenIsSavedAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7311, "acme_revoked_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555101, 555101)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	mod := stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock()))
	rotated := env.Telegram.Rotate(bot.ID) // the owner revoked the token in @BotFather

	require.True(t, env.RunNextStep(t, mod))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, []map[string]any{stepJob(runID, 0)}, queuedSteps(t, env), "the held step is asked again later")
	route, err := env.OutboundModule().Route(ctx, env.DB.Scoped(fixtures.AcmeID), "telegram", new(int64(bot.Integration.ID)))
	require.NoError(t, err)
	assert.Equal(t, outbound.HoldProviderRefused, route.Hold, "the Send route reports the Hold")
	assert.Equal(t, "provider_refused", outbound.HoldProviderRefused)
	assert.Zero(t, env.OutboxCount(t, events.NameTelegramSent, nil))

	// While held, the step does not even ask Telegram.
	asked := len(env.Telegram.RequestsOf(telegramtest.SendMessage))
	require.True(t, env.RunNextStep(t, mod))
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.SendMessage), asked)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
				Kind: siteapi.TelegramConfigInputKindTelegram, Token: rotated,
			}),
		}),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: bot.Integration.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, res)

	env.RunQueuedSteps(t, mod)

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, nil))
}

// A 429 is a Deferral of at least retry_after: the message is not failed, the step is asked again
// later and then sends.
func TestATelegramTooManyRequestsDefersTheStepAndItSendsLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7312, "acme_slow_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555111, 555111)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	mod := stepperWith(env, env.OutboundModule())
	env.Telegram.FailOnce(telegramtest.SendMessage, telegramtest.TooManyRequests(120))
	before := time.Now()

	require.True(t, env.RunNextStep(t, mod))

	assert.Equal(t, automationrun.StatusActive, env.DB.AutomationRun.GetX(ctx, runID).Status)
	jobs := env.JobRowsOf(t, stepping.RunStepArgs{}.Kind())
	require.Len(t, jobs, 1)
	assert.GreaterOrEqual(t, jobs[0].ScheduledAt.Sub(before), 120*time.Second, "not before retry_after")

	env.RunQueuedSteps(t, mod)

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.SendMessage), 2)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, nil))
}

// A 400 about the content fails that Contact's message with Telegram's description, and no other.
func TestATelegramContentRefusalFailsThatMessageWithTelegramsReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7313, "acme_bad_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555121, 555121)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	env.Telegram.Fail(telegramtest.SendMessage, telegramtest.BadEntities)

	err := stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID)

	require.ErrorContains(t, err, "can't parse entities")
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Contains(t, *msg.Reason, "can't parse entities")
	assert.Equal(t, automationrun.StatusFailed, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Equal(t, 1, env.DB.Binding.Query().Where(binding.Destination("555121")).CountX(ctx), "the Binding stays")
}

// Any other 403 (the bot may not start the conversation) will recur on every attempt: it fails that
// message with Telegram's description instead of being retried for good.
func TestATelegramForbiddenThatIsNeitherBlockNorDeactivationFailsTheMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7315, "acme_forbidden_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555141, 555141)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	env.Telegram.Fail(telegramtest.SendMessage, telegramtest.CantInitiate)

	err := stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID)

	require.ErrorContains(t, err, "can't initiate conversation")
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Contains(t, *msg.Reason, "can't initiate conversation")
	assert.Equal(t, 1, env.DB.Binding.Query().Where(binding.Destination("555141")).CountX(ctx), "the Binding stays")
}

// A 403 that is neither block nor deactivation concerns that chat only: the Contact's other chat
// still gets the message, which is then Sent. Only when no chat took it does it fail, with Telegram's
// description. No Binding goes either way.
func TestATelegramForbiddenChatDoesNotKeepTheContactsOtherChatFromTheMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7316, "acme_chat_forbidden_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555151, 555151)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555152, 555152)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	env.Telegram.FailChat(telegramtest.SendMessage, 555151, telegramtest.CantInitiate)

	require.NoError(t, stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID))

	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, map[string]any{"destination": "555152"}))
	assert.Equal(t, 2, env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).CountX(ctx), "no Binding goes")
}

func TestATelegramMessageFailsWhenEveryChatForbidsIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.ConnectTelegram(t, 7317, "acme_all_forbidden_bot")
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555161, 555161)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555162, 555162)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	env.Telegram.Fail(telegramtest.SendMessage, telegramtest.CantInitiate)

	err := stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID)

	require.ErrorContains(t, err, "can't initiate conversation")
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Contains(t, *msg.Reason, "can't initiate conversation")
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.SendMessage), 2, "each chat was tried")
}

// A deactivated user and a missing chat remove the Binding and record telegram.unbound; the other
// Binding of the Contact still gets the message, and with none left the step is Skipped
// no_destination and the sequence goes on.
func TestATelegramDeadChatLosesItsBinding(t *testing.T) {
	for name, gone := range map[string]telegramtest.Failure{
		"deactivated": telegramtest.Deactivated, "chat not found": telegramtest.ChatNotFound,
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			bot := env.ConnectTelegram(t, 7314, "acme_gone_bot")
			automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"), telegramStep(bot, "Again"))
			env.BindTelegram(t, bot, fixtures.ContactBobID, 555131, 555131)
			env.BindTelegram(t, bot, fixtures.ContactBobID, 555132, 555132)
			runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
			mod := stepperWith(env, env.OutboundModule(), stepping.WithClock(passingClock()))
			env.Telegram.FailChat(telegramtest.SendMessage, 555131, gone)

			require.True(t, env.RunNextStep(t, mod))

			left := env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).AllX(ctx)
			require.Len(t, left, 1)
			assert.Equal(t, "555132", left[0].Destination)
			assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramUnbound, map[string]any{"contactId": fixtures.ContactBobID}))
			assert.Equal(t, 1, env.OutboxCount(t, events.NameTelegramSent, map[string]any{"destination": "555132"}), "the other chat got it")

			env.Telegram.FailChat(telegramtest.SendMessage, 555132, gone)
			env.RunQueuedSteps(t, mod)

			assert.Zero(t, env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).CountX(ctx))
			assert.Equal(t, 2, env.OutboxCount(t, events.NameTelegramUnbound, nil))
			skipped := env.DB.OutboundMessage.Query().
				Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(ctx)
			assert.Equal(t, "no_destination", *skipped.Reason)
			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
		})
	}
}
