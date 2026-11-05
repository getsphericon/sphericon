package jobs_test

import (
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var telegramStartLinkIn = regexp.MustCompile(`https://t\.me/[A-Za-z0-9_]+\?start=[A-Za-z0-9_-]+`)

// visitorChat starts the default bot from the tracker's link of the Visitor, as person `from`.
func visitorChat(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot, visitorID string, from int64) {
	t.Helper()
	res, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{VisitorId: visitorID})
	require.NoError(t, err)
	link, ok := res.(*collectapi.CollectTelegramStartLink)
	require.Truef(t, ok, "got %T", res)
	env.StartTelegram(t, bot, from, from, link.URL)
}

func identifyAs(t *testing.T, env *testhelper.TestEnv, visitorID, email string) {
	t.Helper()
	_, err := env.CollectAcme(t).CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
		VisitorId: visitorID, Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(t, err)
}

// An Automation step reaches a Contact through the chat its Visitor started once Identify has
// tied them together.
func TestATelegramStepReachesTheContactAfterIdentifyOfTheVisitorThatStartedTheBot(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	visitorChat(t, env, bot, "dev-step", 555031)
	identifyAs(t, env, "dev-step", fixtures.ContactBobEmail)
	runID := int64(fixtures.AutomationRunJobsTelegramWelcomeBackBobID)
	queueStep(t, env, runID)

	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	assert.Equal(t, "555031", sends[0].String("chat_id"))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSent, msg.Status)
}

// A Visitor's chat with no Contact is nobody's to send to.
func TestAVisitorBindingWithNoContactReachesNobody(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	visitorChat(t, env, bot, "dev-nobody", 555032)
	require.Nil(t, env.DB.Binding.Query().Where(binding.Destination("555032")).OnlyX(t.Context()).ContactID)
	runID := int64(fixtures.AutomationRunJobsTelegramHelloBobID)
	queueStep(t, env, runID)

	runSteps(t, env)

	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).OnlyX(t.Context())
	assert.Equal(t, outboundmessage.StatusSkipped, msg.Status)
	assert.Equal(t, "no_destination", *msg.Reason)
}

// The merge tag works in Telegram text: a fresh personal link on the default bot, which binds
// the chat that starts the bot with it to the recipient.
func TestTheStartTagInATelegramTextIsAPersonalLinkOfTheDefaultBot(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555033, 555033)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramStartTagBobID)
	before := env.DB.StartToken.Query().CountX(t.Context())

	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	link := telegramStartLinkIn.FindString(sends[0].String("text"))
	require.NotEmpty(t, link, sends[0].String("text"))
	assert.Equal(t, before+1, env.DB.StartToken.Query().CountX(t.Context()))
	u, err := url.Parse(link)
	require.NoError(t, err)
	assert.Equal(t, "/"+testhelper.AcmeTelegramUsername, u.Path)
	env.StartTelegram(t, bot, 90, 555034, link)
	bound := env.DB.Binding.Query().Where(binding.Destination("555034")).OnlyX(t.Context())
	require.NotNil(t, bound.ContactID)
	assert.Equal(t, int64(fixtures.ContactBobID), *bound.ContactID)
}

// Without a default bot the tag renders empty, and a text with no tag issues no token.
func TestTheStartTagInATelegramTextIsEmptyWithoutADefaultBotAndUntaggedTextIssuesNoToken(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 555035, 555035)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramStartTagJoinBobID)
	before := env.DB.StartToken.Query().CountX(t.Context())

	runSteps(t, env)

	sends := env.Telegram.RequestsOf(telegramtest.SendMessage)
	require.Len(t, sends, 1)
	assert.Equal(t, "Join: []", sends[0].String("text"))
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()))

	env.MakeDefaultTelegram(t, bot)
	queueStep(t, env, fixtures.AutomationRunJobsTelegramNoTagBobID)

	runSteps(t, env)

	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()))
}
