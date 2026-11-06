package events_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// say is the update Telegram posts when `from` writes text to the bot in a private chat.
func say(updateID, from int64, text string) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, from, from, text)
}

// liamsChat is the Telegram user whose private chat with Acme's fixture bot is bound to Liam
// (BindingLiam).
var liamsChat = lo.Must(strconv.ParseInt(fixtures.BindingLiamDestination, 10, 64))

func TestAMessageFromABoundContactIsRecordedAsReceived(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)

	posted := env.PostTelegramUpdate(t, bot, say(1, liamsChat, "Where is my order?"))
	redelivered := env.PostTelegramUpdate(t, bot, say(1, liamsChat, "Where is my order?"))

	assert.Equal(t, 200, posted.Code)
	assert.Equal(t, 200, redelivered.Code)
	keys := lo.Map(env.QueuedEnvelopes(t, events.NameTelegramReceived), func(e events.Envelope, _ int) string { return e.DedupKey })
	assert.Equal(t, []string{fmt.Sprintf("telegram:%d:1", bot.Integration.ID)}, keys,
		"a redelivered update carries the same key, so the Event is stored and published once")
	asLiam := map[string]any{
		"contactId": fixtures.ContactLiamID, "integrationId": bot.Integration.ID, "text": "Where is my order?",
	}
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramReceived, asLiam))
}

func TestAReceivedTextIsCutAtOneKiB(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	// "é" is two bytes: 600 of them are 1200 bytes, and the cut falls between two characters.
	long := strings.Repeat("é", 600)

	env.PostTelegramUpdate(t, bot, say(2, liamsChat, long))

	got := env.QueuedMessages(t, events.NameTelegramReceived)
	require.Len(t, got, 1)
	assert.Equal(t, strings.Repeat("é", 512), got[0].Data["text"])
}

func TestAMessageFromAnUnknownSenderRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	contacts := env.DB.Contact.Query().CountX(t.Context())

	posted := env.PostTelegramUpdate(t, bot, say(3, 9999, "hello?"))

	assert.Equal(t, 200, posted.Code)
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil))
	assert.Equal(t, contacts, env.DB.Contact.Query().CountX(t.Context()), "no Contact is created")
}

func TestAMessageOnAnotherBotIsNotTheBoundContactsWords(t *testing.T) {
	env := testhelper.Setup(t)
	env.AcmeTelegram(t)
	other := env.AcmeSecondTelegram(t)

	env.PostTelegramUpdate(t, other, say(4, liamsChat, "hi"))

	assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil))
}

func TestACommandWithItsOwnMeaningIsNotReceived(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)

	env.PostTelegramUpdate(t, bot, say(5, liamsChat, "/start"))
	env.PostTelegramUpdate(t, bot, say(6, liamsChat, "/start@"+testhelper.AcmeTelegramUsername+" abc"))
	env.PostTelegramUpdate(t, bot, say(7, liamsChat, "/STOP"))

	assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil))
}

func TestAnotherCommandIsReceivedLikeAnyText(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)

	env.PostTelegramUpdate(t, bot, say(8, liamsChat, "/help me"))

	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramReceived, map[string]any{"text": "/help me"}))
}

// The received Event reaches the consumers like any Event: it enrolls the Contact in the
// Automations it triggers and is delivered to the endpoints subscribed to it.
func TestAReceivedEventTriggersAutomationsAndReachesWebhooks(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.PostTelegramUpdate(t, bot, say(9, liamsChat, "yes please"))
	envelopes := env.QueuedEnvelopes(t, events.NameTelegramReceived)
	require.Len(t, envelopes, 1)
	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).SetEventTypes([]string{events.NameTelegramReceived}).ExecX(t.Context())

	env.PerformEventJobOf(t, jobs.WebhookEventArgs{}, envelopes[0])

	triggers := lo.Filter(env.JobsOf(t, jobkind.EvaluateTrigger.Kind), func(args map[string]any, _ int) bool {
		return args["action"] == events.NameTelegramReceived
	})
	require.Len(t, triggers, 1)
	assert.EqualValues(t, fixtures.ContactLiamID, triggers[0]["contact_id"])
	delivered, ok := lo.Find(env.QueuedDeliveries(t), func(d jobs.DeliverWebhookArgs) bool { return d.EndpointID == fixtures.WebhookCodebasicsID })
	require.True(t, ok, "the subscribed endpoint gets a delivery")
	assert.Equal(t, events.NameTelegramReceived, delivered.EventName)
}
