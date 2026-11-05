package jobs_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/erasure"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Erasing a Contact removes its Telegram data and keeps its refusal (ADR 0021, #962).

// sayTG is the update Telegram posts when `from` writes text to the bot in a private chat.
func sayTG(updateID, from int64, text string) string {
	return fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"date":1767225600,"chat":{"id":%d,"type":"private"},"from":{"id":%d,"is_bot":false,"first_name":"P"},"text":%q}}`,
		updateID, updateID, from, from, text)
}

func eraseContact(t *testing.T, env *testhelper.TestEnv, contactID int64) {
	t.Helper()
	require.NoError(t, erasure.New(env.Bus).Erase(t.Context(), env.DB.Scoped(fixtures.AcmeID),
		erasure.ByContactID(contactID), erasure.Requester{Kind: erasure.RequesterAPIToken}))
}

func TestErasingABoundContactRemovesItsTelegramDataAndKeepsItsOptOut(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	bot := env.AcmeTelegram(t)
	const user = 801001
	step := telegramStep(bot, `See <a href="`+textLink+`">the offer</a>`)
	automationID := telegramAutomation(t, env, step)
	env.BindTelegram(t, bot, fixtures.ContactTelegramErasableID, user, 1)
	enrollVia(t, env, automationID, fixtures.ContactTelegramErasableID)
	runSteps(t, env)
	text, _ := shortPaths(t, env, "801001")
	require.Len(t, text, 1)
	require.Equal(t, http.StatusFound, follow(t, env, text[0], browserUA).Code)
	require.NotZero(t, env.DB.StartToken.Query().Where(starttoken.ContactID(fixtures.ContactTelegramErasableID)).CountX(ctx))

	sent, clicked, bound := int64(fixtures.EventTelegramErasableSentID), int64(fixtures.EventTelegramErasableClickedID), int64(fixtures.EventTelegramErasableBoundID)
	received, otherReceived := int64(fixtures.EventTelegramErasableReceivedID), int64(fixtures.EventTelegramBystanderReceivedID)
	require.Equal(t, http.StatusOK, env.PostTelegramUpdate(t, bot, sayTG(2, user, "STOP")).Code)

	eraseContact(t, env, fixtures.ContactTelegramErasableID)

	assert.Zero(t, env.DB.Binding.Query().Where(binding.ContactID(fixtures.ContactTelegramErasableID)).CountX(ctx), "no Binding")
	assert.Zero(t, env.DB.StartToken.Query().Where(starttoken.ContactID(fixtures.ContactTelegramErasableID)).CountX(ctx), "no Start token")
	assert.Equal(t, http.StatusNotFound, follow(t, env, text[0], browserUA).Code, "the short link answers 404")
	assert.Zero(t, env.DB.TrackedLink.Query().Where(trackedlink.URL(textLink)).CountX(ctx))
	assert.False(t, env.DB.Event.Query().Where(event.ID(received)).ExistX(ctx), "the person's words are gone")
	assert.True(t, env.DB.Event.Query().Where(event.ID(otherReceived)).ExistX(ctx), "another Contact's stay")
	for _, id := range []int64{sent, clicked, bound} {
		e := env.DB.Event.GetX(ctx, id)
		assert.Nilf(t, e.ContactID, "%s keeps no Contact", e.Action)
		assert.Nilf(t, e.Email, "%s keeps no email", e.Action)
		assert.Nilf(t, e.URL, "%s keeps no url", e.Action)
		assert.Emptyf(t, e.Properties, "%s keeps no Destination or url", e.Action)
	}
	kept := env.DB.Unsubscribe.Query().Where(unsubscribe.Destination("801001"), unsubscribe.ChannelEQ(unsubscribe.ChannelTelegram)).AllX(ctx)
	require.Len(t, kept, 1, "the opt-out stays")
	assert.Nil(t, kept[0].ContactID, "detached from the Contact")

	// The same Telegram user is bound to another Contact later (not through /start, which lifts
	// the opt-out in the Channel): nothing is sent to it.
	// test writes: a Binding made without /start, which would lift the opt-out in the Channel.
	_, err := s.Binding().Create().SetIntegrationID(int64(bot.Integration.ID)).SetChannel(binding.ChannelTelegram).
		SetDestination("801001").SetChatID("801001").SetContactID(fixtures.ContactAliceID).SetBoundAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	enrollVia(t, env, automationID, fixtures.ContactAliceID)
	runSteps(t, env)
	assert.Empty(t, env.Telegram.RequestsOf(telegramtest.SendMessage)[1:], "the user is still opted out")
}

func TestErasingAContactRemovesTheBindingsAndStartTokensOfItsVisitors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	const emmaID, emmaPhone = 101, 101
	require.NotZero(t, env.DB.Binding.Query().Where(binding.VisitorID(emmaPhone)).CountX(ctx))
	require.True(t, env.DB.StartToken.Query().Where(starttoken.ID(fixtures.StartTokenEmmaPhoneOpenID)).ExistX(ctx))

	eraseContact(t, env, emmaID)

	assert.Zero(t, env.DB.Binding.Query().Where(binding.VisitorID(emmaPhone)).CountX(ctx), "the Visitor's Binding")
	assert.Zero(t, env.DB.StartToken.Query().Where(starttoken.VisitorID(emmaPhone)).CountX(ctx), "the Visitor's Start token")
	assert.Equal(t, 1, env.DB.Binding.Query().Where(binding.ID(fixtures.BindingLiamID)).CountX(ctx), "another Contact's Binding stays")
}
