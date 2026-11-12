package outboundmessages_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const startLinkBody = `<p>Hi {{ first_name }}</p><p><a href="{{ telegram_start_url }}">Open Telegram</a></p>`

var startLinkIn = regexp.MustCompile(`https://t\.me/[A-Za-z0-9_]+\?start=[A-Za-z0-9_-]+`)

// The tag renders a personal link on the default Telegram bot; whoever starts the bot with it
// binds their chat to the recipient.
func TestAnEmailWithTheStartTagCarriesALinkThatBindsItsRecipient(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	req := withBody(marketing(t, env, "bc:start-link", fixtures.ContactAliceID), startLinkBody)

	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)
	require.Equal(t, outboundmessages.Sent, res.Outcome)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	link := startLinkIn.FindString(msgs[0].HTML)
	require.NotEmpty(t, link, msgs[0].HTML)
	assert.Contains(t, link, "https://t.me/"+testhelper.AcmeTelegramUsername+"?start=")
	assert.Contains(t, msgs[0].Text, link, "the text part carries it too")
	env.StartTelegram(t, bot, 10, 6100, link)
	bound := env.DB.Binding.Query().Where(binding.Destination("6100")).OnlyX(t.Context())
	require.NotNil(t, bound.ContactID)
	assert.Equal(t, int64(fixtures.ContactAliceID), *bound.ContactID)
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramBound, map[string]any{"contactId": fixtures.ContactAliceID}))
}

func TestAnEmailWithoutTheStartTagIssuesNoToken(t *testing.T) {
	env := testhelper.Setup(t)
	env.MakeDefaultTelegram(t, env.AcmeTelegram(t))
	before := env.DB.StartToken.Query().CountX(t.Context())

	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.AcmeID), marketing(t, env, "bc:no-tag", fixtures.ContactAliceID))
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()))
}

// Each send, each render attempt, is a link of its own.
func TestEachSendIssuesAFreshToken(t *testing.T) {
	env := testhelper.Setup(t)
	env.MakeDefaultTelegram(t, env.AcmeTelegram(t))
	before := env.DB.StartToken.Query().CountX(t.Context())

	for _, key := range []string{"bc:fresh-1", "bc:fresh-2"} {
		req := withBody(marketing(t, env, key, fixtures.ContactAliceID), startLinkBody)
		_, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.AcmeID), req)
		require.NoError(t, err)
	}

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 2)
	assert.NotEqual(t, startLinkIn.FindString(msgs[0].HTML), startLinkIn.FindString(msgs[1].HTML))
	assert.Equal(t, before+2, env.DB.StartToken.Query().CountX(t.Context()))
}

// The fixture Workspace has a Telegram Integration, but not a default one.
func TestTheStartTagRendersEmptyWithoutADefaultTelegramBot(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.StartToken.Query().CountX(t.Context())
	req := withBody(marketing(t, env, "bc:no-bot", fixtures.ContactAliceID), `<p>Join: [{{ telegram_start_url }}]</p>`)

	res, err := env.OutboundModule().Send(t.Context(), env.DB.Scoped(fixtures.AcmeID), req)
	require.NoError(t, err)

	assert.Equal(t, outboundmessages.Sent, res.Outcome)
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, "Join: []")
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()))
}

// A Test send is a preview: a placeholder link, and nothing is issued.
func TestAPreviewRendersAPlaceholderAndIssuesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	env.MakeDefaultTelegram(t, env.AcmeTelegram(t))
	before := env.DB.StartToken.Query().CountX(t.Context())

	_, err := env.OutboundModule().SendTest(t.Context(), env.DB.Scoped(fixtures.AcmeID), outboundmessages.TestRequest{
		To: "me@example.com", Content: outboundmessages.EmailContent{Subject: "Preview", Body: startLinkBody},
	})
	require.NoError(t, err)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].HTML, outboundmessages.PreviewStartLink)
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()))
}
