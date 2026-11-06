package jobs_test

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/optout"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/ingesthook"
)

// Opting out and back in on Telegram (ADR 0077): every way of stopping is the person's Channel-wide
// opt-out, keyed by their Telegram user id; the ways back lift it. The bots are the fake Bot API.

const optOutUser = 4101

func said(updateID, from int64, text string) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, from, from, text)
}

func pressed(updateID, from, messageID int64, callbackID, data string) string {
	return fmt.Sprintf(`{"update_id":%d,"callback_query":{"id":%q,"from":{"id":%d,"is_bot":false,"first_name":"P"},"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"}},"chat_instance":"1","data":%q}}`,
		updateID, callbackID, from, messageID, from, data)
}

func memberStatus(updateID, user int64, status string) string {
	return fmt.Sprintf(`{"update_id":%d,"my_chat_member":{"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false},"date":1767225600,"new_chat_member":{"user":{"id":1,"is_bot":true},"status":%q}}}`,
		updateID, user, user, status)
}

// boundBob binds the opt-out user to Bob on Acme's second fixture bot and returns the bot.
func boundBob(t *testing.T, env *testhelper.TestEnv) testhelper.TelegramBot {
	t.Helper()
	bot := env.AcmeSecondTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, optOutUser, 900)
	return bot
}

func mustInt(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	require.NoError(t, err)
	return n
}

func optedOut(t *testing.T, env *testhelper.TestEnv, user int64) bool {
	t.Helper()
	return env.DB.Unsubscribe.Query().Where(
		unsubscribe.ChannelEQ(channels.Telegram), unsubscribe.Destination(strconv.FormatInt(user, 10)),
		unsubscribe.SendingSource(eligibility.SourceEverything),
	).ExistX(t.Context())
}

func TestEachWayToStopIsTheChannelWideOptOutAndTelegramUnsubscribed(t *testing.T) {
	ways := []struct {
		name string
		via  string
		post func(*testing.T, *testhelper.TestEnv, testhelper.TelegramBot)
	}{
		{"/stop", events.UnsubscribedByKeyword, func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, said(1, optOutUser, "/stop"))
		}},
		{"a keyword", events.UnsubscribedByKeyword, func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, said(1, optOutUser, "  Стоп "))
		}},
		{"the button", events.UnsubscribedByButton, func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, pressed(1, optOutUser, 77, "cb-1", "unsubscribe"))
		}},
		{"blocking the bot", events.UnsubscribedByBlock, func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, memberStatus(1, optOutUser, "kicked"))
		}},
	}
	for _, w := range ways {
		t.Run(w.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			bot := boundBob(t, env)

			w.post(t, env, bot)

			assert.True(t, optedOut(t, env, optOutUser))
			row := env.DB.Unsubscribe.Query().Where(
				unsubscribe.ChannelEQ(channels.Telegram), unsubscribe.Destination(strconv.FormatInt(optOutUser, 10)),
			).OnlyX(t.Context())
			assert.Equal(t, w.via, string(lo.FromPtr(row.Via)), "the opt-out keeps how the person stopped")
			recorded := env.QueuedMessages(t, events.NameTelegramUnsubscribed)
			require.Len(t, recorded, 1)
			assert.Equal(t, strconv.Itoa(fixtures.ContactBobID), fmt.Sprint(recorded[0].Data["contactId"]))
			assert.Equal(t, w.via, recorded[0].Data["via"])
			assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil), "a stop is not a message")
		})
	}
}

func TestStoppingTwiceRecordsOneOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)

	env.PostTelegramUpdate(t, bot, said(1, optOutUser, "STOP"))
	env.PostTelegramUpdate(t, bot, said(2, optOutUser, "/stop"))

	assert.Equal(t, 1, env.DB.Unsubscribe.Query().Where(unsubscribe.WorkspaceID(fixtures.AcmeID), unsubscribe.ChannelEQ(channels.Telegram)).CountX(t.Context()))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, nil))
}

func TestAStopFromAnUnknownDestinationIsKeptAndNothingElseIsRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)

	env.PostTelegramUpdate(t, bot, said(1, 9191, "STOP"))

	assert.True(t, optedOut(t, env, 9191))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramUnsubscribed, nil))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil))
}

func TestAStopSkipsTheNextTelegramStepAndTheEmailStepsStillSend(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	automationID := telegramAutomation(t, env,
		telegramStep(bot, "Telegram"),
		automationstep.Step{Type: automationstep.Email, Subject: "Still here", Body: "<p>mail</p>"},
	)
	env.PostTelegramUpdate(t, bot, said(1, optOutUser, "/stop"))
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(t.Context())
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, *skipped.Reason)
	subjects := lo.Map(env.CustomerMail.Messages(), func(m messaging.EmailMessage, _ int) string { return m.Subject })
	assert.Equal(t, []string{"Still here"}, subjects)
}

func TestAStopGivenToOneBotSilencesTheWorkspacesOtherBots(t *testing.T) {
	env := testhelper.Setup(t)
	botA := boundBob(t, env)
	botB := env.AcmeTelegram(t)
	env.BindTelegram(t, botB, fixtures.ContactBobID, optOutUser, 901)
	automationID := telegramAutomation(t, env, telegramStep(botB, "From B"))
	env.PostTelegramUpdate(t, botA, said(1, optOutUser, "STOP"))
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	assert.Equal(t, 1, env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).CountX(t.Context()))
}

func TestUnblockingTheBotAndAnyLaterStartLiftTheOptOut(t *testing.T) {
	lifts := map[string]func(*testing.T, *testhelper.TestEnv, testhelper.TelegramBot){
		"unblocking": func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, memberStatus(2, optOutUser, "member"))
		},
		"a bare /start": func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, said(2, optOutUser, "/start"))
		},
		"a /start with a token the bot does not know": func(t *testing.T, env *testhelper.TestEnv, b testhelper.TelegramBot) {
			t.Helper()
			env.PostTelegramUpdate(t, b, said(2, optOutUser, "/start nope"))
		},
	}
	for name, lift := range lifts {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			bot := boundBob(t, env)
			env.PostTelegramUpdate(t, bot, memberStatus(1, optOutUser, "kicked"))
			require.True(t, optedOut(t, env, optOutUser))

			lift(t, env, bot)

			assert.False(t, optedOut(t, env, optOutUser))
			d, err := eligibility.Check(t.Context(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelTelegram, strconv.Itoa(optOutUser), eligibility.SourceBroadcasts)
			require.NoError(t, err)
			assert.True(t, d.Eligible)
		})
	}
}

func TestADeactivatedUserLosesTheBindingAndHasNoOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	row := env.DB.Integration.GetX(t.Context(), int64(bot.Integration.ID))
	// Telegram sends no update for a deleted account (a send learns it), so the fact is handed to the
	// interpreter as a Preset would hand it.
	res := preset.Result{ReachabilityChanges: []preset.ReachabilityChange{
		{ID: "1", Destination: strconv.Itoa(optOutUser), Reachable: false, Reason: preset.Deactivated},
	}}

	err := ingesthook.New(env.Bus, nil).Interpret(t.Context(), events.Ingest(env.DB.Scoped(fixtures.AcmeID)), row, "telegram", res, time.Now())

	require.NoError(t, err)
	assert.Zero(t, env.DB.Binding.Query().Where(binding.Destination(strconv.Itoa(optOutUser))).CountX(t.Context()))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnbound, map[string]any{"contactId": fixtures.ContactBobID}))
	assert.False(t, optedOut(t, env, optOutUser))
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramUnsubscribed, nil))
}

func TestAPressOnTheUnsubscribeButtonIsAttributedAndAnswered(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	step := telegramStep(bot, "Hello")
	step.UnsubscribeButton = "Stop messages"
	automationID := telegramAutomation(t, env, step)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)
	runSteps(t, env)
	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	assert.JSONEq(t, `{"inline_keyboard":[[{"text":"Stop messages","callback_data":"unsubscribe"}]]}`, sends[0].String("reply_markup"))
	sent := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	require.NotNil(t, sent.ProviderMessageID, "the id Telegram gave the message is kept to resolve a press")
	messageID := sent.ProviderMessageID

	posted := env.PostTelegramUpdate(t, bot, pressed(5, optOutUser, mustInt(t, *messageID), "cb-77", "unsubscribe"))

	assert.Equal(t, 200, posted.Code)
	assert.True(t, optedOut(t, env, optOutUser))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{
		"contactId": fixtures.ContactBobID, "automationId": automationID, "via": events.UnsubscribedByButton,
	}), "attributed to the Automation the message came from")
	answers := env.Telegram.RequestsOf(telegramtest.AnswerCallbackQuery)
	require.Len(t, answers, 1)
	assert.Equal(t, "cb-77", answers[0].String("callback_query_id"))
	assert.Equal(t, bot.Token, answers[0].Token)
}

func TestAPressOnAnotherButtonIsAnsweredAndChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)

	env.PostTelegramUpdate(t, bot, pressed(5, optOutUser, 9, "cb-9", "something-else"))

	assert.False(t, optedOut(t, env, optOutUser))
	assert.Len(t, env.Telegram.RequestsOf(telegramtest.AnswerCallbackQuery), 1)
}

func TestAnAnswerThatFailsDoesNotFailTheRecordedPress(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	env.Telegram.Fail(telegramtest.AnswerCallbackQuery, telegramtest.BadRequest("query is too old"))

	posted := env.PostTelegramUpdate(t, bot, pressed(5, optOutUser, 9, "cb-old", "unsubscribe"))

	assert.Equal(t, 200, posted.Code)
	assert.True(t, optedOut(t, env, optOutUser))
}

func TestABlockedSendIsTheOptOutAndTheBindingIsNotAccepted(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 4102, 902)
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.Telegram.FailChat(telegramtest.SendMessage, optOutUser, telegramtest.Blocked)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	assert.True(t, optedOut(t, env, optOutUser))
	assert.False(t, optedOut(t, env, 4102))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{"via": events.UnsubscribedByBlock, "automationId": automationID}))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
	assert.Equal(t, "4102", msg.Destination, "the first Binding that accepted it, not the blocked one")
}

func TestASendToOnlyABlockedChatIsSkippedAsUnsubscribed(t *testing.T) {
	env := testhelper.Setup(t)
	bot := boundBob(t, env)
	automationID := telegramAutomation(t, env, telegramStep(bot, "Hello"))
	env.Telegram.FailChat(telegramtest.SendMessage, optOutUser, telegramtest.Blocked)
	runID := enrollVia(t, env, automationID, fixtures.ContactBobID)

	runSteps(t, env)

	assert.True(t, optedOut(t, env, optOutUser))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSkipped, msg.Status)
	assert.Equal(t, eligibility.ReasonUnsubscribedEverything, *msg.Reason)
}

func TestARecordedOptOutByHandOnTelegramCoversTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	boundBob(t, env)
	c := env.ExternalScoped(t, "contacts:write")

	created, err := c.UnsubscribesCreate(t.Context(), &externalapi.CreateUnsubscribeInput{
		Channel:     externalapi.NewOptConsentChannel(externalapi.ConsentChannelTelegram),
		Destination: strconv.Itoa(optOutUser),
	})

	require.NoError(t, err)
	assert.Equal(t, externalapi.ConsentChannelTelegram, created.Channel)
	assert.Equal(t, eligibility.SourceEverything, created.SendingSource)
	assert.True(t, optedOut(t, env, optOutUser))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{"via": events.UnsubscribedByManual}))
}

func TestAManualTelegramSuppressionStopsEverySendToTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "suppressions:write")

	created, err := c.SuppressionsCreate(t.Context(), &externalapi.CreateSuppressionInput{
		Channel:     externalapi.NewOptSuppressionChannel(externalapi.SuppressionChannelTelegram),
		Destination: strconv.Itoa(optOutUser),
	})

	require.NoError(t, err)
	assert.Equal(t, externalapi.SuppressionChannelTelegram, created.Channel)
	d, err := eligibility.Check(t.Context(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelTelegram, strconv.Itoa(optOutUser), "")
	require.NoError(t, err)
	assert.Equal(t, eligibility.ReasonSuppressed, d.Reason)
}

func TestAManualTelegramDestinationThatIsNoUserIdIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "suppressions:write")

	_, err := c.SuppressionsCreate(t.Context(), &externalapi.CreateSuppressionInput{
		Channel:     externalapi.NewOptSuppressionChannel(externalapi.SuppressionChannelTelegram),
		Destination: "@alice",
	})

	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

// stopFrom is Bob stopping in Telegram from a message of Acme that came from broadcastID or
// automationID, as a button press or a blocked send records it (optout.StopTelegram).
func stopFrom(t *testing.T, env *testhelper.TestEnv, broadcastID, automationID int64) {
	t.Helper()
	msg := &ent.OutboundMessage{ContactID: lo.ToPtr(int64(fixtures.ContactBobID))}
	if broadcastID != 0 {
		msg.BroadcastID = &broadcastID
	}
	if automationID != 0 {
		msg.AutomationID = &automationID
	}
	err := env.Bus.WithinScopedTx(t.Context(), env.DB.Scoped(fixtures.AcmeID), func(ts *ent.Scoped, pub events.Publisher) error {
		return optout.StopTelegram(t.Context(), ts, pub, optout.TelegramStop{
			Destination: strconv.Itoa(optOutUser), Via: events.UnsubscribedByButton, Message: msg,
			Now: time.Now(), DedupKey: "test:stop",
		})
	})
	require.NoError(t, err)
}

// The Broadcast counts a Telegram stop made from its message when the broadcasts consumer works
// telegram.unsubscribed: the recipient row is the broadcasts module's (ADR 0102), and a repeated
// job counts nothing more.
func TestATelegramStopOfABroadcastsMessageCountsTheUnsubscribeOnceItsConsumerRuns(t *testing.T) {
	env := testhelper.Setup(t)
	boundBob(t, env)

	stopFrom(t, env, fixtures.BroadcastArchivedID, 0)
	before := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientParkedBobID).UnsubscribedAt
	env.WorkBroadcastUnsubscribeJobs(t)
	counted := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientParkedBobID).UnsubscribedAt
	env.WorkBroadcastUnsubscribeJobs(t)
	again := env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientParkedBobID).UnsubscribedAt

	assert.Nil(t, before, "the opt-out's transaction leaves the count to the consumer")
	require.NotNil(t, counted)
	require.NotNil(t, again)
	assert.True(t, counted.Equal(*again), "counted once")
}

func TestATelegramStopOfAMessageWithoutABroadcastCountsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	boundBob(t, env)

	stopFrom(t, env, 0, fixtures.AutomationArchivedID)
	env.WorkBroadcastUnsubscribeJobs(t)

	assert.Empty(t, env.JobsOf(t, jobs.BroadcastUnsubscribeEventArgs{}.Kind()))
	assert.Nil(t, env.DB.BroadcastRecipient.GetX(t.Context(), fixtures.BroadcastRecipientParkedBobID).UnsubscribedAt)
}
