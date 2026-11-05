package site_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func buttonsOf(b ...siteapi.TelegramButton) []siteapi.TelegramButton { return b }

// A Telegram Broadcast on the site API (ADR 0080): its content is a text, the Channel an email
// Broadcast's Integration is refused for, no Test send, and figures without open or delivery.

func TestSiteTelegramBroadcastIsCreatedWithItsTextAndReportsFiguresPerChannel(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:          "News",
		Content:       testhelper.SiteTelegram("Hello <b>{{ first_name }}</b>"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	made, ok := created.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", created)
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
	require.NoError(t, err)
	read, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)

	assert.Equal(t, siteapi.BroadcastChannelTelegram, read.Channel)
	assert.Equal(t, "Hello <b>{{ first_name }}</b>", testhelper.SiteTelegramOf(read).Text)
	assert.Empty(t, made.Warnings, "email's lint does not apply to a text")
	st := read.Stats
	assert.True(t, st.OpenedCount.Null)
	assert.True(t, st.DeliveredCount.Null)
	assert.True(t, st.BouncedCount.Null)
	assert.True(t, st.ComplainedCount.Null)
	assert.True(t, st.CapturedCount.Null)
	assert.True(t, st.DeliveryRate.Null)
	assert.True(t, st.OpenRate.Null)
	assert.True(t, st.ClickToOpenRate.Null)
	assert.Zero(t, st.PendingCount)
}

func TestSiteTelegramBroadcastRefusesAnEmailIntegrationAndAContentOfTheOtherChannel(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	onEmail, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "News", Content: testhelper.SiteTelegram("Hello"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeDefaultID)),
	}, params)
	require.NoError(t, err)
	res, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "News", Content: testhelper.SiteTelegram("Hello"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
	}, params)
	require.NoError(t, err)
	b, ok := res.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", res)
	asEmail, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Elsewhere")}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, onEmail, "an email Integration never sends a Telegram Broadcast")
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, asEmail, "a Broadcast is never converted to another Channel")
}

func TestSiteTelegramBroadcastScheduleChecksTheRouteAndTheWrittenLengthAndHasNoTestSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	botID := idStr(int64(bot.Integration.ID))
	create := func(text string, integration *siteapi.EntityId) siteapi.EntityId {
		in := &siteapi.CreateBroadcastInput{Name: "News", Content: testhelper.SiteTelegram(text)}
		if integration != nil {
			in.IntegrationId = siteapi.NewOptNilEntityId(*integration)
		}
		res, err := c.SiteBroadcastsCreate(ctx, in, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		b, ok := res.(*siteapi.SiteBroadcastResource)
		require.Truef(t, ok, "got %T", res)
		return b.ID
	}
	schedule := func(id siteapi.EntityId) siteapi.SiteBroadcastsScheduleRes {
		res, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
			siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
		require.NoError(t, err)
		return res
	}

	noDefault := schedule(create("Hello", nil))
	tooLong := schedule(create(strings.Repeat("é", 4097), &botID))
	empty := schedule(create("  ", &botID))
	okID := create("Hello", &botID)
	scheduled := schedule(okID)
	test, err := c.SiteBroadcastsTestSend(ctx, &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("john@acme.test")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: okID})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteBroadcastsScheduleConflict{}, noDefault)
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, tooLong)
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, empty, "a blank text would fail every recipient")
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled)
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, test)
	assert.Empty(t, env.CustomerMail.Messages(), "no Test send went out")
}

// The Message content of a step is the Broadcast's: it is stored and read back whole, an edit that
// leaves a field out keeps it, and a blank image, an empty list of buttons and a blank unsubscribe
// label remove theirs.
func TestSiteTelegramBroadcastCarriesTheMessageContentOfAStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	botID := idStr(int64(bot.Integration.ID))
	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "News", IntegrationId: siteapi.NewOptNilEntityId(botID),
		Content: testhelper.SiteTelegramContent(siteapi.TelegramBroadcastContentInput{
			Text:              siteapi.NewOptString("Hi"),
			ImageUrl:          siteapi.NewOptString("https://cdn.example.com/a.png"),
			Buttons:           buttonsOf(siteapi.TelegramButton{Label: "Open", URL: "https://example.com/a"}),
			LinkPreview:       siteapi.NewOptBool(false),
			UnsubscribeButton: siteapi.NewOptString("Stop"),
		}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	made, ok := created.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", created)
	read := func() siteapi.TelegramBroadcastContent {
		got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
		require.NoError(t, err)
		b, ok := got.(*siteapi.SiteBroadcastResource)
		require.Truef(t, ok, "got %T", got)
		return testhelper.SiteTelegramOf(b)
	}
	edit := func(in siteapi.TelegramBroadcastContentInput) siteapi.SiteBroadcastsUpdateRes {
		res, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteTelegramContentEdit(in)},
			siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
		require.NoError(t, err)
		return res
	}

	stored := read()
	assert.Equal(t, "https://cdn.example.com/a.png", stored.ImageUrl.Value)
	assert.Equal(t, buttonsOf(siteapi.TelegramButton{Label: "Open", URL: "https://example.com/a"}), stored.Buttons)
	assert.False(t, stored.LinkPreview)
	assert.Equal(t, "Stop", stored.UnsubscribeButton.Value)

	edit(siteapi.TelegramBroadcastContentInput{Text: siteapi.NewOptString("Hello")})
	kept := read()
	assert.Equal(t, "Hello", kept.Text)
	assert.Equal(t, stored.ImageUrl, kept.ImageUrl, "a field left out keeps its value")
	assert.Equal(t, stored.Buttons, kept.Buttons)
	assert.False(t, kept.LinkPreview)

	edit(siteapi.TelegramBroadcastContentInput{
		ImageUrl: siteapi.NewOptString(""), Buttons: []siteapi.TelegramButton{}, LinkPreview: siteapi.NewOptBool(true),
		UnsubscribeButton: siteapi.NewOptString(" "),
	})
	cleared := read()
	assert.Empty(t, cleared.ImageUrl.Value, "a blank image removes it")
	assert.Empty(t, cleared.Buttons)
	assert.True(t, cleared.LinkPreview)
	assert.Empty(t, cleared.UnsubscribeButton.Value, "a blank label removes the button")

	badImage := edit(siteapi.TelegramBroadcastContentInput{ImageUrl: siteapi.NewOptString("http://example.com/a.png")})
	badButton := edit(siteapi.TelegramBroadcastContentInput{Buttons: buttonsOf(siteapi.TelegramButton{Label: "x", URL: "javascript:1"})})
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, badImage, "an image is an https URL")
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, badButton, "a button URL is http or https")
}

// Schedule applies the rule an Automation's Publish applies to a step: the text as written is
// 4096 long at most, 1024 with an image; a message with no text and no image has nothing to send.
func TestSiteTelegramBroadcastScheduleKnowsTheCaptionLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	scheduled := func(text, image string) siteapi.SiteBroadcastsScheduleRes {
		in := siteapi.TelegramBroadcastContentInput{Text: siteapi.NewOptString(text)}
		if image != "" {
			in.ImageUrl = siteapi.NewOptString(image)
		}
		res, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
			Name: "News", IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
			Content: testhelper.SiteTelegramContent(in),
		}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		b, ok := res.(*siteapi.SiteBroadcastResource)
		require.Truef(t, ok, "got %T", res)
		out, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
			siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
		require.NoError(t, err)
		return out
	}
	const image = "https://cdn.example.com/a.png"

	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled(strings.Repeat("a", 4096), ""))
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, scheduled(strings.Repeat("a", 1025), image), "a caption is 1024 at most")
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled(strings.Repeat("a", 1024), image))
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, scheduled("", image), "an image alone is a message")
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, scheduled(" ", ""), "no text and no image")
}
