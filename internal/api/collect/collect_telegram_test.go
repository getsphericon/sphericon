package collect_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	entcontact "github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func startLink(t *testing.T, env *testhelper.TestEnv, visitorID string) *collectapi.CollectTelegramStartLink {
	t.Helper()
	link, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{VisitorId: visitorID})
	require.NoError(t, err)
	return link
}

func identify(t *testing.T, env *testhelper.TestEnv, visitorID, email string) {
	t.Helper()
	_, err := env.CollectAcme(t).CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
		VisitorId: visitorID, Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(t, err)
}

// contactChats is the Telegram chats the Contact with the email is reached through, as its card
// lists them.
func contactChats(t *testing.T, env *testhelper.TestEnv, email string) []siteapi.SiteBindingResource {
	t.Helper()
	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email(email)).OnlyX(t.Context())
	chats, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteTelegramListBindings(t.Context(), siteapi.SiteTelegramListBindingsParams{
		WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contact.ID),
	})
	require.NoError(t, err)
	return chats
}

// The tracker's link binds the chat that starts the bot to the Visitor, with no Event. An Identify
// through the public collect key proves nothing (ADR 0171), so the chat stays the Visitor's and
// reaches no Contact.
func TestAVisitorStartLinkBindsTheChatToTheVisitorAndAClaimedIdentifyLeavesItThere(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)

	link := startLink(t, env, "dev-tg")
	env.StartTelegram(t, bot, 1, 6001, link.URL)
	identify(t, env, "dev-tg", "tracker.person@example.com")

	assert.Contains(t, link.URL, "https://t.me/"+testhelper.AcmeTelegramUsername+"?start=")
	vis := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-tg")).OnlyX(t.Context())
	owned := env.DB.Binding.Query().Where(binding.Destination("6001")).OnlyX(t.Context())
	require.NotNil(t, owned.VisitorID)
	assert.Equal(t, vis.ID, *owned.VisitorID, "the chat is the Visitor's")
	assert.Nil(t, owned.ContactID)
	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramBound, nil), "a Visitor's Binding records no Event")
	assert.Empty(t, contactChats(t, env, "tracker.person@example.com"), "the claimed Contact is not reached")
}

// The order does not matter: a chat started after the claimed Identify is the Visitor's as well.
func TestAChatStartedAfterAClaimedIdentifyIsTheVisitors(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)

	identify(t, env, "dev-after", "after.person@example.com")
	env.StartTelegram(t, bot, 2, 6002, startLink(t, env, "dev-after").URL)

	owned := env.DB.Binding.Query().Where(binding.Destination("6002")).OnlyX(t.Context())
	assert.Nil(t, owned.ContactID)
	assert.Empty(t, contactChats(t, env, "after.person@example.com"))
	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramBound, nil))
}

func TestAVisitorStartLinkIsRefusedWithoutAnEnabledDefaultBot(t *testing.T) {
	env := testhelper.Setup(t)

	before := env.DB.StartToken.Query().CountX(t.Context())

	_, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{VisitorId: "dev-nobot"})

	testhelper.CollectRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()), "no token is issued")
}

// A message from a Visitor's chat is nobody's, before a claimed Identify and after it: no
// telegram.received is recorded.
func TestAMessageFromAVisitorsChatIsNobodysEvenAfterAClaimedIdentify(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 3, 6003, startLink(t, env, "dev-received").URL)

	env.PostTelegramUpdate(t, bot, `{"update_id":4,"message":{"message_id":4,"date":1767225600,"chat":{"id":6003,"type":"private"},"from":{"id":6003,"is_bot":false},"text":"before"}}`)
	identify(t, env, "dev-received", "received.person@example.com")
	env.PostTelegramUpdate(t, bot, `{"update_id":5,"message":{"message_id":5,"date":1767225600,"chat":{"id":6003,"type":"private"},"from":{"id":6003,"is_bot":false},"text":"after"}}`)

	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramReceived, nil))
}

// A Visitor's chat that a link issued for a Contact starts again becomes that Contact's: it is
// announced as bound to it, and nobody lost it.
func TestAVisitorsChatStartedAgainByAContactsLinkBecomesTheContacts(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 8, 6005, startLink(t, env, "dev-same").URL)
	identify(t, env, "dev-same", "same.person@example.com")
	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email("same.person@example.com")).OnlyX(t.Context())

	env.BindTelegram(t, bot, contact.ID, 6005, 9)

	assert.Zero(t, env.QueuedCount(t, systemevent.TelegramUnbound, nil))
	assert.Equal(t, 1, env.QueuedCount(t, systemevent.TelegramBound, map[string]any{"contactId": contact.ID}))
	assert.Len(t, contactChats(t, env, "same.person@example.com"), 1)
	assert.Equal(t, 1, env.DB.Binding.Query().Where(binding.Destination("6005")).CountX(t.Context()))
}
