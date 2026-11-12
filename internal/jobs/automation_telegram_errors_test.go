package jobs_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// What Telegram answers at send time is sorted by whom it concerns (ADR 0079): the bot, the rate,
// the message or the chat. The bot is the fake Bot API, scripted to refuse.

// A 401 holds the bot: the Enrollment waits at its step with the Hold, nothing is recorded as sent,
// and saving the Integration with a token Telegram accepts lets the same step send.
func TestATelegramUnauthorizedHoldsTheBotUntilItsTokenIsSavedAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555101, 555101)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
	mod := stepperAt(env, env.OutboundModule(), passingClock())
	rotated := env.Telegram.Rotate(bot.ID) // the owner revoked the token in @BotFather

	require.True(t, env.RunNextStep(t, mod))

	run := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the held step is asked again later")
	route, err := env.OutboundModule().Route(ctx, env.DB.Scoped(fixtures.AcmeID), "telegram", new(int64(bot.Integration.ID)))
	require.NoError(t, err)
	assert.Equal(t, holdreason.ProviderRefused, route.Hold, "the Send route reports the Hold")
	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramSent, nil))

	// While held, the step does not even ask Telegram.
	asked := len(env.Telegram.RequestsOf(telegramtest.SendMessage))
	require.True(t, env.RunNextStep(t, mod))
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.SendMessage), asked)

	_, err = env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(ctx, t, int64(bot.Integration.ID))),
		Config: siteapi.NewOptIntegrationConfigInput(siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
				Kind: siteapi.TelegramConfigInputKindTelegram, Token: rotated,
			}),
		}),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: bot.Integration.ID})
	require.NoError(t, err)

	env.RunQueuedSteps(t, mod)

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, nil))
}

// A bot Telegram refused at a send and accepts again (the Owner unblocked it on Telegram's side) is
// saved with its same token, through either API: the token is checked with Telegram anew, the hold
// goes and the waiting step sends.
func TestAHeldTelegramIntegrationSavedWithTheSameTokenSendsAgain(t *testing.T) {
	cases := map[string]struct {
		save func(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) error
	}{
		"site": {save: func(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) error {
			t.Helper()
			_, err := saveTelegramToken(t, env, bot)
			return err
		}},
		"external": {save: func(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) error {
			t.Helper()
			_, err := saveTelegramTokenExternal(t, env, bot)
			return err
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			bot, runID, mod := heldTelegram(t, env)
			env.Telegram.Clear()

			err := tc.save(t, env, bot)

			require.NoError(t, err)
			hold := holdOf(t, env, bot)
			assert.Empty(t, hold, "the hold is lifted")
			env.RunQueuedSteps(t, mod)
			done := env.DB.AutomationRun.GetX(t.Context(), runID)
			assert.Equal(t, automationrun.StatusCompleted, done.Status)
			sent := env.QueuedCount(t, systemevent.TelegramSent, nil)
			assert.Equal(t, 1, sent)
		})
	}
}

// When Telegram still refuses the token, saving it again is refused as a bad token is, and the
// Integration stays held: nothing is sent.
func TestAHeldTelegramIntegrationSavedWithATokenTelegramStillRefusesStaysHeld(t *testing.T) {
	env := testhelper.Setup(t)
	bot, runID, mod := heldTelegram(t, env)
	env.Telegram.Clear()
	env.Telegram.Revoke(bot.Token)

	_, err := saveTelegramToken(t, env, bot)

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Errors.Value, "config.token")
	hold := holdOf(t, env, bot)
	assert.Equal(t, holdreason.ProviderRefused, hold)
	require.True(t, env.RunNextStep(t, mod))
	waiting := env.DB.AutomationRun.GetX(t.Context(), runID)
	assert.Equal(t, automationrun.StatusActive, waiting.Status)
	sent := env.QueuedCount(t, systemevent.TelegramSent, nil)
	assert.Zero(t, sent)
}

// Saving an Integration that is not held with its same token changes nothing and does not ask
// Telegram: the save succeeds even while Telegram would refuse the token.
func TestATelegramIntegrationNotHeldSavedWithTheSameTokenChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.Telegram.Fail(telegramtest.GetMe, telegramtest.Unauthorized)

	saved, err := saveTelegramToken(t, env, bot)

	require.NoError(t, err)
	assert.Equal(t, bot.Integration.Config, saved.Config, "the bot is the one connected")
	hold := holdOf(t, env, bot)
	assert.Empty(t, hold)
}

// heldTelegram connects a bot whose Enrollment waits at a step held by Telegram's 401 on the send:
// the token itself stays known to Telegram, which refuses sends until the test clears the script.
func heldTelegram(t *testing.T, env *testhelper.TestEnv) (testhelper.TelegramBot, int64, *automations.StepExecution) {
	t.Helper()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555101, 555101)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
	mod := stepperAt(env, env.OutboundModule(), passingClock())
	env.Telegram.Fail(telegramtest.SendMessage, telegramtest.Unauthorized)
	require.True(t, env.RunNextStep(t, mod))
	hold := holdOf(t, env, bot)
	require.Equal(t, holdreason.ProviderRefused, hold)
	return bot, runID, mod
}

// holdOf is the Hold the Send route reports for the bot's Integration, empty when it sends.
func holdOf(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) holdreason.Reason {
	t.Helper()
	route, err := env.OutboundModule().Route(t.Context(), env.DB.Scoped(fixtures.AcmeID), "telegram", new(int64(bot.Integration.ID)))
	require.NoError(t, err)
	return route.Hold
}

// saveTelegramToken saves the Integration with its same token through the Site API, as an Owner
// does once the bot works again, and returns the answer.
func saveTelegramToken(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) (*siteapi.SiteIntegrationResource, error) {
	t.Helper()
	return env.SiteActor(t, fixtures.OwnerJohnEmail).SiteIntegrationsUpdate(t.Context(), &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(t.Context(), t, int64(bot.Integration.ID))),
		Config: siteapi.NewOptIntegrationConfigInput(siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
				Kind: siteapi.TelegramConfigInputKindTelegram, Token: bot.Token,
			}),
		}),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: bot.Integration.ID})
}

// saveTelegramTokenExternal is saveTelegramToken through the External API.
func saveTelegramTokenExternal(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot) (*externalapi.IntegrationResource, error) {
	t.Helper()
	return env.ExternalScoped(t, apitokens.ScopeIntegrationsWrite).IntegrationsUpdate(t.Context(), &externalapi.UpdateIntegrationInput{Revision: externalapi.Revision(env.IntegrationRevision(t.Context(), t, int64(externalapi.EntityId(bot.Integration.ID)))),
		Config: externalapi.NewOptIntegrationConfigInput(externalapi.IntegrationConfigInput{
			OneOf: externalapi.NewTelegramConfigInputIntegrationConfigInputSum(externalapi.TelegramConfigInput{
				Kind: externalapi.TelegramConfigInputKindTelegram, Token: bot.Token,
			}),
		}),
	}, externalapi.IntegrationsUpdateParams{ID: externalapi.EntityId(bot.Integration.ID)})
}

// A 429 is a Deferral of at least retry_after: the message is not failed, the step is asked again
// later and then sends.
func TestATelegramTooManyRequestsDefersTheStepAndItSendsLater(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555111, 555111)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
	mod := stepperWith(env, env.OutboundModule())
	env.Telegram.FailOnce(telegramtest.SendMessage, telegramtest.TooManyRequests(120))
	before := time.Now()

	require.True(t, env.RunNextStep(t, mod))

	assert.Equal(t, automationrun.StatusActive, env.DB.AutomationRun.GetX(ctx, runID).Status)
	jobs := env.JobRowsOf(t, automations.RunStepArgs{}.Kind())
	require.Len(t, jobs, 1)
	assert.GreaterOrEqual(t, jobs[0].ScheduledAt.Sub(before), 120*time.Second, "not before retry_after")

	env.RunQueuedSteps(t, mod)

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.SendMessage), 2)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, nil))
}

// A 400 about the content fails that Contact's message with Telegram's description, and no other.
func TestATelegramContentRefusalFailsThatMessageWithTelegramsReason(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555121, 555121)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
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
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555141, 555141)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
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
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555151, 555151)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555152, 555152)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
	env.Telegram.FailChat(telegramtest.SendMessage, 555151, telegramtest.CantInitiate)

	require.NoError(t, stepOnce(t, env, stepperWith(env, env.OutboundModule()), runID))

	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, map[string]any{"destination": "555152"}))
	assert.Equal(t, 2, env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).CountX(ctx), "no Binding goes")
}

func TestATelegramMessageFailsWhenEveryChatForbidsIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555161, 555161)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555162, 555162)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)
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
			bot := env.AcmeTelegram(t)
			env.BindTelegram(t, bot, fixtures.ContactBobID, 555131, 555131)
			env.BindTelegram(t, bot, fixtures.ContactBobID, 555132, 555132)
			runID := int64(fixtures.AutomationRunJobsTelegramHelloAgainBobID)
			queueStep(t, env, runID)
			mod := stepperAt(env, env.OutboundModule(), passingClock())
			env.Telegram.FailChat(telegramtest.SendMessage, 555131, gone)

			require.True(t, env.RunNextStep(t, mod))

			left := env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).AllX(ctx)
			require.Len(t, left, 1)
			assert.Equal(t, "555132", left[0].Destination)
			assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramUnbound, map[string]any{"contactId": fixtures.ContactBobID}))
			assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramSent, map[string]any{"destination": "555132"}), "the other chat got it")

			env.Telegram.FailChat(telegramtest.SendMessage, 555132, gone)
			env.RunQueuedSteps(t, mod)

			assert.Zero(t, env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactBobID)).CountX(ctx))
			assert.Equal(t, 2, env.QueuedCount(t, systemevent.TelegramUnbound, nil))
			skipped := env.DB.OutboundMessage.Query().
				Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(ctx)
			assert.Equal(t, "no_destination", *skipped.Reason)
			assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
		})
	}
}
