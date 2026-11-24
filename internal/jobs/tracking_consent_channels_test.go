package jobs_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SMS and Telegram follow Engagement tracking consent as email does (ADR 0120): a message to a
// Destination that is not tracked keeps its links as written, and a click of a message whose
// Destination refused since redirects and records nothing.

// lacunaTracking records an Engagement tracking observation of a Lacuna Destination through /api,
// as the customer does.
func lacunaTracking(t *testing.T, env *testhelper.TestEnv, channel externalapi.EngagementTrackingChannel, destination string, state externalapi.EngagementTrackingState) {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.LacunaID, apitokens.ScopeContactsWrite))
	_, err := c.EngagementTrackingCreate(t.Context(), &externalapi.RecordEngagementTrackingInput{Channel: channel, Destination: destination, State: state})
	require.NoError(t, err)
}

// An SMS Broadcast with link tracking on to a number that refused tracking goes out with its links
// as written, its opt-out link kept.
func TestAnSMSToANumberThatRefusedTrackingKeepsItsLinksAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, fixtures.ContactLacunaBeaPhone, externalapi.EngagementTrackingStateDenied)

	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].Body, "Sale: "+smsOffer+" or your cart "+smsCart+". Stop: ", "the links as written")
	assert.Contains(t, sent[0].Body, "/e/u/", "the opt-out link stays")
	assert.Empty(t, smsShortLink.FindAllString(sent[0].Body, -1), "no short tracked link")
}

// An SMS whose author kept the links as written goes out untracked whatever the consent: a grant
// of its number changes nothing of it.
func TestAnSMSWithLinkTrackingOffGoesOutAsWrittenWhateverTheConsent(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, fixtures.ContactLacunaBeaPhone, externalapi.EngagementTrackingStateGranted)

	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsUntrackedID))

	sent := env.Twilio.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, "Sale: "+smsOffer+" or "+smsCart, sent[0].Body)
}

// A refusal recorded after an SMS went out reaches it: its Short tracked link still redirects and
// records nothing.
func TestARefusalAfterAnSMSWentOutRecordsNothingOfItsClick(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTwilio(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaSmsLinksID))
	paths := smsShortPaths(t, env)
	require.Len(t, paths, 2, "the message went out tracked")

	lacunaTracking(t, env, externalapi.EngagementTrackingChannelSMS, fixtures.ContactLacunaBeaPhone, externalapi.EngagementTrackingStateDenied)
	resp := follow(t, env, paths[1], browserUA)

	assert.Equal(t, http.StatusFound, resp.Code, "the link redirects")
	assert.Equal(t, smsCart, resp.Header().Get("Location"))
	clicks := env.QueuedCount(t, systemevent.SmsClicked, nil)
	assert.Zero(t, clicks, "no click is recorded")
}

// A Telegram message to a chat that refused tracking keeps its text links, its button URLs and its
// unsubscribe button as written; a chat that did not refuse gets Short tracked links.
func TestATelegramMessageToAChatThatRefusedTrackingKeepsItsLinksAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTelegram(t)
	lacunaTracking(t, env, externalapi.EngagementTrackingChannelTelegram, benChat, externalapi.EngagementTrackingStateDenied)

	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramRichID))

	bens := requestsTo(env.Telegram.RequestsOf(telegramtest.SendMessage), benChat)
	require.Len(t, bens, 1)
	assert.Equal(t, `<a href="https://shop.test/offer?a=1&amp;b=2">offer</a>`, bens[0].String("text"), "the text link as written")
	assert.JSONEq(t, `{"inline_keyboard":[[{"text":"Open","url":"`+buttonLink+`"}]]}`, bens[0].String("reply_markup"), "the button URL as written")
	photos := requestsTo(env.Telegram.RequestsOf(telegramtest.SendPhoto), benChat)
	require.Len(t, photos, 1)
	assert.JSONEq(t, `{"inline_keyboard":[[{"text":"Open","url":"https://example.com/a"}],[{"callback_data":"unsubscribe","text":"Stop news"}]]}`,
		photos[0].String("reply_markup"), "the unsubscribe button stays")
	text, buttons := shortPaths(t, env, fayChat)
	assert.Len(t, text, 1, "a chat that did not refuse gets Short tracked links")
	assert.Len(t, buttons, 1)
}

// A refusal recorded after a Telegram message went out reaches it: its Short tracked link still
// redirects and records nothing.
func TestARefusalAfterATelegramMessageWentOutRecordsNothingOfItsClick(t *testing.T) {
	env := testhelper.Setup(t)
	env.LacunaTelegram(t)
	require.NoError(t, env.SendBroadcastThrough(t, env.OutboundModule(), fixtures.BroadcastLacunaTelegramClicksID))
	text, _ := shortPaths(t, env, benChat)
	require.Len(t, text, 1, "the message went out tracked")

	lacunaTracking(t, env, externalapi.EngagementTrackingChannelTelegram, benChat, externalapi.EngagementTrackingStateDenied)
	resp := follow(t, env, text[0], browserUA)

	assert.Equal(t, http.StatusFound, resp.Code, "the link redirects")
	assert.Equal(t, textLink, resp.Header().Get("Location"))
	clicks := env.QueuedCount(t, systemevent.TelegramClicked, nil)
	assert.Zero(t, clicks, "no click is recorded")
}
