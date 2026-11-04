package collect_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	entcontact "github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/visitor"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func startLink(t *testing.T, env *testhelper.TestEnv, visitorID string) *collectapi.CollectTelegramStartLink {
	t.Helper()
	res, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{VisitorId: visitorID})
	require.NoError(t, err)
	link, ok := res.(*collectapi.CollectTelegramStartLink)
	require.Truef(t, ok, "got %T", res)
	return link
}

func identify(t *testing.T, env *testhelper.TestEnv, visitorID, email string) {
	t.Helper()
	_, err := env.CollectAcme(t).CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
		VisitorId: visitorID, Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(t, err)
}

// The tracker's link binds the chat that starts the bot to the Visitor, with no Event; Identify
// then makes the chat the Contact's and records telegram.bound once, however often it repeats.
func TestAVisitorStartLinkBindsTheChatToTheVisitorAndIdentifyAnnouncesIt(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7600, "acme_tracker_bot")
	env.MakeDefaultTelegram(t, bot)

	link := startLink(t, env, "dev-tg")
	env.StartTelegram(t, bot, 1, 6001, link.URL)

	assert.Contains(t, link.URL, "https://t.me/acme_tracker_bot?start=")
	vis := env.DB.Visitor.Query().Where(visitor.WorkspaceID(fixtures.AcmeID), visitor.VisitorID("dev-tg")).OnlyX(t.Context())
	owned := env.DB.Binding.Query().Where(binding.Destination("6001")).OnlyX(t.Context())
	require.NotNil(t, owned.VisitorID)
	assert.Equal(t, vis.ID, *owned.VisitorID, "the chat is the Visitor's")
	assert.Nil(t, owned.ContactID)
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramBound, nil), "a Visitor's Binding records no Event")

	identify(t, env, "dev-tg", "tracker.person@example.com")
	identify(t, env, "dev-tg", "tracker.person@example.com")

	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email("tracker.person@example.com")).OnlyX(t.Context())
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramBound, map[string]any{"contactId": contact.ID, "integrationId": bot.Integration.ID}),
		"announced for the Contact once")
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteTelegramListBindings(t.Context(), siteapi.SiteTelegramListBindingsParams{
		WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contact.ID),
	})
	require.NoError(t, err)
	chats, ok := res.(*siteapi.SiteTelegramListBindingsOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	require.Len(t, *chats, 1, "the Contact is reached through its Visitor")
	assert.Equal(t, "acme_tracker_bot", (*chats)[0].BotUsername.Or(""))
}

// A Visitor's chat is announced to the next Contact the Visitor resolves to as well.
func TestAVisitorBindingIsAnnouncedToAnotherContactItIsIdentifiedAs(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7601, "acme_shared_bot")
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 2, 6002, startLink(t, env, "dev-shared").URL)

	identify(t, env, "dev-shared", "first.person@example.com")
	identify(t, env, "dev-shared", "second.person@example.com")

	assert.Equal(t, 2, env.QueuedCount(t, events.NameTelegramBound, map[string]any{"integrationId": bot.Integration.ID}))
}

func TestAVisitorStartLinkIsRefusedWithoutAnEnabledDefaultBot(t *testing.T) {
	env := testhelper.Setup(t)

	before := env.DB.StartToken.Query().CountX(t.Context())

	res, err := env.CollectAcme(t).CollectTelegramStartLinksCreate(t.Context(), &collectapi.CollectTelegramStartLinkInput{VisitorId: "dev-nobot"})

	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectTelegramStartLinksCreateUnprocessableEntity{}, res)
	assert.Equal(t, before, env.DB.StartToken.Query().CountX(t.Context()), "no token is issued")
}

// A message from a chat bound through a Visitor is the Contact's once the Visitor is identified:
// telegram.received is recorded for it.
func TestAMessageFromAChatBoundThroughAnIdentifiedVisitorIsTheContactsMessage(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7602, "acme_received_bot")
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 3, 6003, startLink(t, env, "dev-received").URL)

	env.PostTelegramUpdate(t, bot, `{"update_id":4,"message":{"message_id":4,"date":1767225600,"chat":{"id":6003,"type":"private"},"from":{"id":6003,"is_bot":false},"text":"before"}}`)
	assert.Zero(t, env.QueuedCount(t, events.NameTelegramReceived, nil), "a Visitor with no Contact has none to record it for")

	identify(t, env, "dev-received", "received.person@example.com")
	env.PostTelegramUpdate(t, bot, `{"update_id":5,"message":{"message_id":5,"date":1767225600,"chat":{"id":6003,"type":"private"},"from":{"id":6003,"is_bot":false},"text":"after"}}`)

	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email("received.person@example.com")).OnlyX(t.Context())
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramReceived, map[string]any{"contactId": contact.ID}))
}

// A chat that moves from an identified Visitor to another owner is lost by the Contact the Visitor
// was identified as: telegram.unbound is recorded for it.
func TestAVisitorsChatMovingAwayIsAnnouncedAsUnboundToItsContact(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7603, "acme_moved_bot")
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 6, 6004, startLink(t, env, "dev-from").URL)
	identify(t, env, "dev-from", "from.person@example.com")

	env.StartTelegram(t, bot, 7, 6004, startLink(t, env, "dev-to").URL)

	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email("from.person@example.com")).OnlyX(t.Context())
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnbound, map[string]any{"contactId": contact.ID}))
}

// A chat bound through a Visitor that Identify resolved to a Contact, which then starts the bot
// with a link issued for that Contact itself, loses and gains nothing: no telegram.unbound.
func TestAChatStartedAgainAsTheContactItsVisitorBecameIsNotUnbound(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.ConnectTelegram(t, 7604, "acme_same_bot")
	env.MakeDefaultTelegram(t, bot)
	env.StartTelegram(t, bot, 8, 6005, startLink(t, env, "dev-same").URL)
	identify(t, env, "dev-same", "same.person@example.com")
	contact := env.DB.Contact.Query().Where(entcontact.WorkspaceID(fixtures.AcmeID), entcontact.Email("same.person@example.com")).OnlyX(t.Context())

	env.BindTelegram(t, bot, contact.ID, 6005, 9)

	assert.Zero(t, env.QueuedCount(t, events.NameTelegramUnbound, map[string]any{"contactId": contact.ID}))
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramBound, map[string]any{"contactId": contact.ID}), "announced once, by Identify")
	assert.Equal(t, 1, env.DB.Binding.Query().Where(binding.Destination("6005")).CountX(t.Context()))
}
