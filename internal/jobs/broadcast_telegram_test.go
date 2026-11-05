package jobs_test

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/broadcastrecipient"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/telegramrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Telegram Broadcast (ADR 0080) goes to the Contacts with an eligible Binding on its bot, one
// message per Binding, and reports figures of its own. The bot is the fake Bot API, which the
// Outbound module of the harness reaches as production does.

// telegramBroadcast creates a draft Telegram Broadcast through the bot with text, to all contacts.
func telegramBroadcast(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot, text string) int64 {
	t.Helper()
	return telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{Text: &text})
}

// telegramBroadcastOf is telegramBroadcast with the whole Message content given.
func telegramBroadcastOf(t *testing.T, env *testhelper.TestEnv, bot testhelper.TelegramBot, content broadcasts.TelegramContentInput) int64 {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	b, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).Create(t.Context(), s, broadcasts.CreateInput{
		Name:          new("Telegram news"),
		Content:       content,
		IntegrationID: new(int64(bot.Integration.ID)),
	})
	require.NoError(t, err)
	return b.ID
}

// requestsTo is the requests made to one chat.
func requestsTo(reqs []telegramtest.Request, chatID string) []telegramtest.Request {
	return lo.Filter(reqs, func(r telegramtest.Request, _ int) bool { return r.String("chat_id") == chatID })
}

func chatsSentTo(env *testhelper.TestEnv) []string {
	return lo.Map(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) string { return r.String("chat_id") })
}

func TestATelegramBroadcastFreezesOnlyBoundContactsAndSendsOncePerBinding(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	other := env.ConnectTelegram(t, 7402, "acme_other_bot")
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600001, 1)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600002, 2)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600003, 3)
	env.BindTelegram(t, other, fixtures.ContactCarolID, 600004, 4)
	id := telegramBroadcast(t, env, bot, "Hi {{ first_name }}, <b>news</b>")

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).Order(broadcastrecipient.ByContactID()).AllX(ctx)
	assert.Equal(t, []int64{fixtures.ContactAliceID, fixtures.ContactBobID, fixtures.ContactLiamID, fixtures.ContactEmmaID},
		lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID }),
		"only the Contacts bound on this bot (the fixture binds Liam, and Emma by her phone), once each however many Bindings")
	assert.ElementsMatch(t, []string{"600001", "600002", "600003", "70001", "70002"}, chatsSentTo(env), "one message per Binding")
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, id).Status)
	messages := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id)).AllX(ctx)
	assert.Len(t, messages, 4, "one Outbound message per Contact")
	figures := figuresOf(t, env, id)
	assert.Equal(t, 4, figures.Recipients)
	assert.Equal(t, 4, figures.Sent)
	assert.Zero(t, figures.Pending)
}

func TestATelegramBroadcastToAContactWhoStoppedIsNotFrozenAndALostBindingIsSkipped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600011, 11)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600012, 12)
	env.BindTelegram(t, bot, fixtures.ContactCarolID, 600013, 13)
	env.UnsubscribeTelegramFromBroadcasts(ctx, "600012")
	id := telegramBroadcast(t, env, bot, "Hello")
	mod := env.OutboundModule()

	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	// Alice's only Binding is gone between the freeze and the send.
	env.DB.Binding.Delete().Where(binding.Destination("600011")).ExecX(ctx)
	for _, rid := range ids {
		require.NoError(t, deliverRecipient(ctx, env.DB, mod, rid))
	}

	assert.Len(t, ids, 4, "Bob stopped, so he is not a recipient; the fixture's Liam and Emma are")
	assert.ElementsMatch(t, []string{"600013", "70001", "70002"}, chatsSentTo(env))
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(ctx)
	assert.Equal(t, "no_destination", *skipped.Reason)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 4, Sent: 3, Skipped: 1}, figures)
}

func TestATelegramBroadcastPacesAtALoweredProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	for i := range int64(3) {
		env.BindTelegram(t, bot, fixtures.ContactAliceID+i, 600021+i, 21+i)
	}
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := telegramBroadcast(t, env, bot, "Slowly")
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.AcmeID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 5, "three bound here, and the fixture's Liam and Emma")
	for i := 1; i < len(times); i++ {
		assert.Equal(t, 500*time.Millisecond, times[i].Sub(times[i-1]), "two a second is 500ms apart")
	}
	assert.True(t, env.DB.Broadcast.GetX(ctx, id).LastScheduledAt.Equal(times[len(times)-1].Truncate(time.Microsecond)), "the progress estimate reads the last scheduled send")
}

// The pacer spaces sends by the calls they make: a Contact with two chats takes two steps, so the
// next Contact starts one second after it at two calls a second, and the bucket never goes negative.
// Alice has two chats; Bob, and the fixture's Liam and Emma, one each.
func TestATelegramBroadcastPaceReservesOneStepPerChatOfARecipient(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600081, 81)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600082, 82)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600083, 83)
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := telegramBroadcast(t, env, bot, "Two chats")
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.AcmeID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	require.Len(t, ids, 4)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 4)
	chats := map[int64]int{fixtures.ContactAliceID: 2}
	// Whoever goes first starts at once, and each next one waits one 500ms step per call the one
	// before it makes.
	assert.Equal(t, time.Duration(0), times[0].Sub(start))
	for i := 1; i < len(ids); i++ {
		calls := max(chats[env.DB.BroadcastRecipient.GetX(ctx, ids[i-1]).ContactID], 1)
		assert.Equal(t, time.Duration(calls)*500*time.Millisecond, times[i].Sub(times[i-1]))
	}
}

func TestATelegramBroadcastToASegmentFreezesOnlyItsBoundContacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, 103, 600031, 31) // plan: pro
	env.BindTelegram(t, bot, 102, 600032, 32) // plan: free
	id := telegramBroadcast(t, env, bot, "Pro news")
	env.DB.Broadcast.UpdateOneID(id).SetSegmentID(fixtures.SegmentProPlanID).ExecX(ctx)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).AllX(ctx)
	assert.ElementsMatch(t, []int64{fixtures.ContactLiamID, 103}, lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID }),
		"the pro-plan Contacts bound on the bot: 103 here and the fixture's Liam")
	assert.ElementsMatch(t, []string{"600031", "70001"}, chatsSentTo(env))
}

// A Binding owned by a Visitor reaches the Contact that Visitor belongs to (ADR 0075).
func TestATelegramBroadcastReachesAContactThroughItsVisitorsBinding(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	b, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).Create(ctx, s, broadcasts.CreateInput{
		Name:          new("Fixture bot"),
		Content:       broadcasts.TelegramContentInput{Text: new("Hi")},
		IntegrationID: new(int64(fixtures.IntegrationAcmeTelegramID)),
	})
	require.NoError(t, err)

	size, err := broadcasts.AudienceSize(ctx, s, b)

	require.NoError(t, err)
	assert.Equal(t, 2, size, "Liam by his own Binding, Emma by her phone's")
}

// A Telegram Broadcast has the Message content of a step and goes through the same send path: an
// image sends a photo with the text as its caption, link buttons and the unsubscribe button make the
// keyboard, the link preview switch sets the option.
func TestATelegramBroadcastSendsItsImageButtonsAndUnsubscribeButton(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600041, 41)
	id := telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{
		Text:              new("Look <b>here</b>"),
		ImageURL:          new("https://cdn.example.com/a.png"),
		Buttons:           &[]telegramrender.Button{{Label: "Open", URL: "https://example.com/a"}},
		UnsubscribeButton: new("Stop news"),
	})
	textOnly := telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{Text: new("See https://example.com"), LinkPreview: new(false)})

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), textOnly))

	photos := requestsTo(env.Telegram.RequestsOf(telegramtest.SendPhoto), "600041")
	require.Len(t, photos, 1)
	assert.Equal(t, "https://cdn.example.com/a.png", photos[0].String("photo"))
	assert.Equal(t, "Look <b>here</b>", photos[0].String("caption"))
	assert.Regexp(t,
		`^\{"inline_keyboard":\[\[\{"text":"Open","url":"https?://[^"/]+/c/[A-Za-z0-9_-]+"\}\],\[\{"callback_data":"unsubscribe","text":"Stop news"\}\]\]\}$`,
		photos[0].String("reply_markup"), "the link button is a short link (ADR 0083)")
	sends := requestsTo(env.Telegram.RequestsOf(telegramtest.SendMessage), "600041")
	require.Len(t, sends, 1)
	assert.JSONEq(t, `{"is_disabled":true}`, sends[0].String("link_preview_options"))
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, id).Status)
}

// A caption over 1024 once rendered fails that message alone, as it does in a step.
func TestATelegramBroadcastCaptionOverTheCaptionLimitFailsThatMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600051, 51)
	env.DB.Contact.UpdateOneID(fixtures.ContactBobID).SetFirstName(fmt.Sprintf("%0100d", 0)).ExecX(ctx)
	id := telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{
		Text: new(fmt.Sprintf("%0950d{{ first_name }}", 0)), ImageURL: new("https://cdn.example.com/a.png"),
	})

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	assert.Empty(t, requestsTo(env.Telegram.RequestsOf(telegramtest.SendPhoto), "600051"))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.ContactID(fixtures.ContactBobID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Equal(t, outbound.ReasonContentTooLong, *msg.Reason)
}

// {{ unsubscribe_url }} in a Broadcast's text opts the chat out of the "broadcasts" source on
// Telegram only, and a press on the button is attributed to the Broadcast.
func TestATelegramBroadcastsUnsubscribeLinkAndButtonOptTheChatOutOfBroadcasts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	id := telegramBroadcastOf(t, env, bot, broadcasts.TelegramContentInput{
		Text: new(`<a href="{{ unsubscribe_url }}">stop</a>`), UnsubscribeButton: new("Stop"),
	})
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	sends := requestsTo(env.Telegram.RequestsOf(telegramtest.SendMessage), "70001") // Liam's fixture chat
	require.Len(t, sends, 1)
	link := regexp.MustCompile(`/e/u/([^"]+)"`).FindStringSubmatch(sends[0].String("text"))
	require.Len(t, link, 2, "the text carries the unsubscribe link")

	_, err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})
	require.NoError(t, err)

	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ("70001")).OnlyX(ctx)
	assert.Equal(t, unsubscribe.ChannelTelegram, optOut.Channel)
	assert.Equal(t, eligibility.SourceBroadcasts, optOut.SendingSource)
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{"broadcastId": id, "via": events.UnsubscribedByLink}))
}

// A bot whose token Telegram refuses holds the Broadcast with provider_refused: the recipient waits
// pending, the Broadcast shows the reason, and saving a token Telegram accepts lets it send and
// lifts the hold.
func TestATelegramBroadcastIsHeldWhileItsBotIsRefusedAndResumesWhenTheTokenIsSavedAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	id := telegramBroadcast(t, env, bot, "Hello")
	mod := env.OutboundModule()
	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	require.Len(t, ids, 2, "the fixture's Liam, and Emma by her phone")
	rotated := env.Telegram.Rotate(bot.ID) // the owner revoked the token in @BotFather

	err = deliverRecipient(ctx, env.DB, mod, ids[0])

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outbound.HoldProviderRefused, held.Reason)
	assert.True(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[0]), broadcasts.Pending()).ExistX(ctx))
	assert.Equal(t, outbound.HoldProviderRefused, string(*env.DB.Broadcast.GetX(ctx, id).HoldReason))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	shown, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, siteapi.NewOptNilHoldReason(siteapi.HoldReasonProviderRefused), shown.HoldReason, "the Broadcast shows why it waits")

	res, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
				Kind: siteapi.TelegramConfigInputKindTelegram, Token: rotated,
			}),
		}),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: bot.Integration.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, res)
	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))

	assert.False(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[0]), broadcasts.Pending()).ExistX(ctx))
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, id).HoldReason, "the hold clears when a send goes through")
	accepted := lo.Filter(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) bool { return r.Token == rotated })
	require.Len(t, accepted, 1, "the one send the new token made")
	assert.Contains(t, []string{"70001", "70002"}, accepted[0].String("chat_id"), "to the first recipient's fixture chat")
}

// The Provider limit counts bot calls, not messages: a Contact with two chats spends two tokens,
// so at two a second the next Contact's send waits.
func TestATelegramContactWithSeveralChatsSpendsTheProviderLimitPerChat(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600041, 41)
	env.BindTelegram(t, bot, fixtures.ContactAliceID, 600042, 42)
	env.BindTelegram(t, bot, fixtures.ContactBobID, 600043, 43)
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := telegramBroadcast(t, env, bot, "Paced")
	mod := env.OutboundModule()
	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	require.Len(t, ids, 4, "Alice, Bob, and the fixture's Liam and Emma")

	for _, rid := range ids {
		// The ones after Alice are deferred: its error is the retry.
		if err := deliverRecipient(ctx, env.DB, mod, rid); err != nil {
			t.Log(err)
		}
	}

	assert.ElementsMatch(t, []string{"600041", "600042"}, chatsSentTo(env), "the others wait: Alice's two chats took the two tokens")
}
