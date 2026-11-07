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
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram/telegramtest"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Telegram Broadcast (ADR 0080) goes to the Contacts with an eligible Binding on its bot, one
// message per Binding, and reports figures of its own. The bot is the fake Bot API, which the
// Outbound module of the harness reaches as production does.

// The chats of Lacuna's fixture Bindings on its bot (IntegrationLacunaTelegram): Tia's two, Ben's,
// Eve's (her Visitor's) and Fay's. Sid's chat there is one he stopped Broadcasts in, and Cal's is
// on the other bot.
const (
	tiaChat      = "77401"
	tiaOtherChat = "77402"
	benChat      = "77403"
	eveChat      = "77404"
	fayChat      = "77405"
)

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
	env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramHelloID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).Order(broadcastrecipient.ByContactID()).AllX(ctx)
	assert.Equal(t, []int64{fixtures.ContactLacunaTwoChatsID, fixtures.ContactLacunaBenID, fixtures.ContactLacunaByVisitorID, fixtures.ContactLacunaFayID},
		lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID }),
		"only the Contacts bound on this bot (Eve by her Visitor's chat; not Sid, who stopped, nor Cal, on the other bot), once each however many Bindings")
	assert.ElementsMatch(t, []string{tiaChat, tiaOtherChat, benChat, eveChat, fayChat}, chatsSentTo(env), "one message per Binding")
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
	env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramHelloID)
	mod := env.OutboundModule()

	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	// Ben's only Binding is gone between the freeze and the send.
	env.DB.Binding.Delete().Where(binding.Destination(benChat)).ExecX(ctx)
	for _, rid := range ids {
		require.NoError(t, deliverRecipient(ctx, env.DB, mod, rid))
	}

	assert.Len(t, ids, 4, "Sid stopped, so he is not a recipient; Tia, Ben, Eve and Fay are")
	assert.ElementsMatch(t, []string{tiaChat, tiaOtherChat, eveChat, fayChat}, chatsSentTo(env))
	skipped := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.StatusEQ(outboundmessage.StatusSkipped)).OnlyX(ctx)
	assert.Equal(t, int64(fixtures.ContactLacunaBenID), lo.FromPtr(skipped.ContactID))
	assert.Equal(t, "no_destination", *skipped.Reason)
	figures := figuresOf(t, env, id)
	assert.Equal(t, broadcasts.Stats{Recipients: 4, Sent: 3, Skipped: 1}, figures)
}

// Lacuna's Segment Chatham holds Ben, Eve and Fay, each in one chat of the bot.
func TestATelegramBroadcastPacesAtALoweredProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaTelegramChathamID)
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.LacunaID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 3, "Ben, Eve and Fay")
	assert.Equal(t, 500*time.Millisecond, times[1].Sub(times[0]), "two a second is 500ms apart")
	assert.Equal(t, 500*time.Millisecond, times[2].Sub(times[1]))
	assert.True(t, env.DB.Broadcast.GetX(ctx, id).LastScheduledAt.Equal(times[2].Truncate(time.Microsecond)), "the progress estimate reads the last scheduled send")
}

// The pacer spaces sends by the calls they make: a Contact with two chats takes two steps, so the
// next Contact starts one second after it at two calls a second, and the bucket never goes negative.
// Tia has two chats; Ben, Eve and Fay one each.
func TestATelegramBroadcastPaceReservesOneStepPerChatOfARecipient(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaTelegramHelloID)
	del := broadcasts.NewDelivery(env.OutboundModule())
	s := env.DB.Scoped(fixtures.LacunaID)
	start := time.Now()

	ids, err := del.Plan(ctx, s, id)
	require.NoError(t, err)
	require.Len(t, ids, 4)
	times, err := del.Pace(ctx, s, id, ids, start)
	require.NoError(t, err)

	require.Len(t, times, 4)
	chats := map[int64]int{fixtures.ContactLacunaTwoChatsID: 2}
	// Whoever goes first starts at once, and each next one waits one 500ms step per call the one
	// before it makes.
	assert.Equal(t, time.Duration(0), times[0].Sub(start))
	for i := 1; i < len(ids); i++ {
		calls := max(chats[env.DB.BroadcastRecipient.GetX(ctx, ids[i-1]).ContactID], 1)
		assert.Equal(t, time.Duration(calls)*500*time.Millisecond, times[i].Sub(times[i-1]))
	}
}

// Lacuna's Segment Chatham holds Ben, Eve and Fay, bound on the bot, and Cal, bound only on the
// other one; Tia, bound on the bot, lives elsewhere.
func TestATelegramBroadcastToASegmentFreezesOnlyItsBoundContacts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramChathamID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	recipients := env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.BroadcastID(id)).AllX(ctx)
	assert.ElementsMatch(t, []int64{fixtures.ContactLacunaBenID, fixtures.ContactLacunaByVisitorID, fixtures.ContactLacunaFayID},
		lo.Map(recipients, func(r *ent.BroadcastRecipient, _ int) int64 { return r.ContactID }),
		"the Segment's Contacts bound on the bot")
	assert.ElementsMatch(t, []string{benChat, eveChat, fayChat}, chatsSentTo(env))
}

// A Binding owned by a Visitor reaches the Contact that Visitor belongs to (ADR 0075).
func TestATelegramBroadcastReachesAContactThroughItsVisitorsBinding(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	b := env.DB.Broadcast.UpdateOneID(fixtures.BroadcastTelegramDraftID).SetIntegrationID(fixtures.IntegrationAcmeTelegramID).SaveX(ctx)

	size, err := broadcasts.AudienceSize(ctx, s, b, time.Now())

	require.NoError(t, err)
	assert.Equal(t, 2, size, "Liam by his own Binding, Emma by her phone's")
}

// A Telegram Broadcast has the Message content of a step and goes through the same send path: an
// image sends a photo with the text as its caption, link buttons and the unsubscribe button make the
// keyboard, the link preview switch sets the option.
func TestATelegramBroadcastSendsItsImageButtonsAndUnsubscribeButton(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramRichID)
	textOnly := int64(fixtures.BroadcastLacunaTelegramNoPreviewID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), textOnly))

	photos := requestsTo(env.Telegram.RequestsOf(telegramtest.SendPhoto), benChat)
	require.Len(t, photos, 1)
	assert.Equal(t, "https://cdn.example.com/a.png", photos[0].String("photo"))
	assert.Equal(t, "Look <b>here</b>", photos[0].String("caption"))
	assert.Regexp(t,
		`^\{"inline_keyboard":\[\[\{"text":"Open","url":"https?://[^"/]+/c/[A-Za-z0-9_-]+"\}\],\[\{"callback_data":"unsubscribe","text":"Stop news"\}\]\]\}$`,
		photos[0].String("reply_markup"), "the link button is a short link (ADR 0083)")
	sends := requestsTo(env.Telegram.RequestsOf(telegramtest.SendMessage), benChat)
	require.Len(t, sends, 1)
	assert.JSONEq(t, `{"is_disabled":true}`, sends[0].String("link_preview_options"))
	assert.Equal(t, broadcast.StatusSent, env.DB.Broadcast.GetX(ctx, id).Status)
}

// A caption over 1024 once rendered fails that message alone, as it does in a step: Lacuna's draft
// is 950 zeros and the first name, Ben's made 100 characters long.
func TestATelegramBroadcastCaptionOverTheCaptionLimitFailsThatMessage(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTelegram(t)
	env.DB.Contact.UpdateOneID(fixtures.ContactLacunaBenID).SetFirstName(fmt.Sprintf("%0100d", 0)).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaTelegramLongCaptionID)

	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))

	assert.Empty(t, requestsTo(env.Telegram.RequestsOf(telegramtest.SendPhoto), benChat))
	msg := env.DB.OutboundMessage.Query().Where(outboundmessage.BroadcastID(id), outboundmessage.ContactID(fixtures.ContactLacunaBenID)).OnlyX(ctx)
	assert.Equal(t, outboundmessage.StatusFailed, msg.Status)
	assert.Equal(t, outboundmessages.ReasonContentTooLong, *msg.Reason)
}

// {{ unsubscribe_url }} in a Broadcast's text opts the chat out of the "broadcasts" source on
// Telegram only, and a press on the button is attributed to the Broadcast.
func TestATelegramBroadcastsUnsubscribeLinkAndButtonOptTheChatOutOfBroadcasts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramUnsubscribeID)
	require.NoError(t, jobs.SendBroadcast(ctx, env.DB, env.OutboundModule(), id))
	sends := requestsTo(env.Telegram.RequestsOf(telegramtest.SendMessage), benChat)
	require.Len(t, sends, 1)
	link := regexp.MustCompile(`/e/u/([^"]+)"`).FindStringSubmatch(sends[0].String("text"))
	require.Len(t, link, 2, "the text carries the unsubscribe link")

	err := env.SiteAnonymous(t).SitePublicUnsubscribesPerform(ctx, siteapi.SitePublicUnsubscribesPerformParams{Token: link[1]})
	require.NoError(t, err)

	optOut := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(benChat)).OnlyX(ctx)
	assert.Equal(t, channels.Telegram, optOut.Channel)
	assert.Equal(t, eligibility.SourceBroadcasts, optOut.SendingSource)
	assert.Equal(t, 1, env.QueuedCount(t, events.NameTelegramUnsubscribed, map[string]any{"broadcastId": id, "via": events.UnsubscribedByLink}))
}

// A bot whose token Telegram refuses holds the Broadcast with provider_refused: the recipient waits
// pending, the Broadcast shows the reason, and saving a token Telegram accepts lets it send and
// lifts the hold.
func TestATelegramBroadcastIsHeldWhileItsBotIsRefusedAndResumesWhenTheTokenIsSavedAgain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	id := int64(fixtures.BroadcastLacunaTelegramChathamID)
	mod := env.OutboundModule()
	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	require.Len(t, ids, 3, "Chatham's Ben, Eve and Fay, one chat each")
	rotated := env.Telegram.Rotate(bot.ID) // the owner revoked the token in @BotFather

	err = deliverRecipient(ctx, env.DB, mod, ids[0])

	var held *broadcasts.HeldError
	require.ErrorAs(t, err, &held)
	assert.Equal(t, outboundmessages.HoldProviderRefused, held.Reason)
	assert.True(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[0]), broadcasts.Pending()).ExistX(ctx))
	assert.Equal(t, outboundmessages.HoldProviderRefused, string(*env.DB.Broadcast.GetX(ctx, id).HoldReason))
	c := env.SiteActor(t, fixtures.LacunaOwnerLenaEmail)
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.LacunaPublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	shown := got
	assert.Equal(t, siteapi.NewOptNilHoldReason(siteapi.HoldReasonProviderRefused), shown.HoldReason, "the Broadcast shows why it waits")

	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(siteapi.IntegrationConfigInput{
			OneOf: siteapi.NewTelegramConfigInputIntegrationConfigInputSum(siteapi.TelegramConfigInput{
				Kind: siteapi.TelegramConfigInputKindTelegram, Token: rotated,
			}),
		}),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.LacunaPublicID, ID: bot.Integration.ID})
	require.NoError(t, err)

	require.NoError(t, deliverRecipient(ctx, env.DB, mod, ids[0]))

	assert.False(t, env.DB.BroadcastRecipient.Query().Where(broadcastrecipient.ID(ids[0]), broadcasts.Pending()).ExistX(ctx))
	assert.Nil(t, env.DB.Broadcast.GetX(ctx, id).HoldReason, "the hold clears when a send goes through")
	accepted := lo.Filter(env.Telegram.RequestsOf(telegramtest.SendMessage), func(r telegramtest.Request, _ int) bool { return r.Token == rotated })
	require.Len(t, accepted, 1, "the one send the new token made")
	assert.Contains(t, []string{benChat, eveChat, fayChat}, accepted[0].String("chat_id"), "to the first recipient's chat")
}

// The Provider limit counts bot calls, not messages: a Contact with two chats spends two tokens,
// so at two a second the next Contact's send waits. The Instance clock stands still, so no token
// refills while the sends run, however long they take.
func TestATelegramContactWithSeveralChatsSpendsTheProviderLimitPerChat(t *testing.T) {
	still := time.Now()
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return still }))
	ctx := t.Context()
	bot := env.LacunaTelegram(t)
	env.DB.Integration.UpdateOneID(int64(bot.Integration.ID)).SetMaxPerSecond(2).ExecX(ctx)
	id := int64(fixtures.BroadcastLacunaTelegramHelloID)
	mod := env.OutboundModule()
	ids, err := planAudience(ctx, env.DB, mod, id)
	require.NoError(t, err)
	require.Len(t, ids, 4, "Tia, Ben, Eve and Fay")

	errs := lo.Map(ids, func(rid int64, _ int) error { return deliverRecipient(ctx, env.DB, mod, rid) })

	require.NoError(t, errs[0], "Tia goes first")
	deferred := lo.CountBy(errs[1:], func(err error) bool { return err != nil })
	assert.Equal(t, 3, deferred, "the ones after Tia are deferred: the error is the retry")
	assert.ElementsMatch(t, []string{tiaChat, tiaOtherChat}, chatsSentTo(env), "the others wait: Tia's two chats took the two tokens")
}
