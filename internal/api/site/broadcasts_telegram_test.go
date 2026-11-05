package site_test

import (
	"net/http"
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

	made, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:          "News",
		Content:       testhelper.SiteTelegram("Hello <b>{{ first_name }}</b>"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
	require.NoError(t, err)
	read := got

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

	_, onEmail := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "News", Content: testhelper.SiteTelegram("Hello"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeDefaultID)),
	}, params)
	b, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "News", Content: testhelper.SiteTelegram("Hello"),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
	}, params)
	require.NoError(t, err)
	_, asEmail := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Elsewhere")}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})

	testhelper.SiteRefusal(t, onEmail, http.StatusUnprocessableEntity, "an email Integration never sends a Telegram Broadcast")
	testhelper.SiteRefusal(t, asEmail, http.StatusUnprocessableEntity, "a Broadcast is never converted to another Channel")
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
		b, err := c.SiteBroadcastsCreate(ctx, in, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		return b.ID
	}
	schedule := func(id siteapi.EntityId) error {
		_, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
			siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
		return err
	}

	noDefault := schedule(create("Hello", nil))
	tooLong := schedule(create(strings.Repeat("é", 4097), &botID))
	empty := schedule(create("  ", &botID))
	okID := create("Hello", &botID)
	scheduled := schedule(okID)
	_, err := c.SiteBroadcastsTestSend(ctx, &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("john@acme.test")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: okID})

	testhelper.SiteRefusal(t, noDefault, http.StatusConflict)
	testhelper.SiteRefusal(t, tooLong, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, empty, http.StatusUnprocessableEntity, "a blank text would fail every recipient")
	require.NoError(t, scheduled)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
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
	made, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
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
	read := func() siteapi.TelegramBroadcastContent {
		b, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
		require.NoError(t, err)
		return testhelper.SiteTelegramOf(b)
	}
	edit := func(in siteapi.TelegramBroadcastContentInput) error {
		_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteTelegramContentEdit(in)},
			siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: made.ID})
		return err
	}

	stored := read()
	assert.Equal(t, "https://cdn.example.com/a.png", stored.ImageUrl.Value)
	assert.Equal(t, buttonsOf(siteapi.TelegramButton{Label: "Open", URL: "https://example.com/a"}), stored.Buttons)
	assert.False(t, stored.LinkPreview)
	assert.Equal(t, "Stop", stored.UnsubscribeButton.Value)

	require.NoError(t, edit(siteapi.TelegramBroadcastContentInput{Text: siteapi.NewOptString("Hello")}))
	kept := read()
	assert.Equal(t, "Hello", kept.Text)
	assert.Equal(t, stored.ImageUrl, kept.ImageUrl, "a field left out keeps its value")
	assert.Equal(t, stored.Buttons, kept.Buttons)
	assert.False(t, kept.LinkPreview)

	require.NoError(t, edit(siteapi.TelegramBroadcastContentInput{
		ImageUrl: siteapi.NewOptString(""), Buttons: []siteapi.TelegramButton{}, LinkPreview: siteapi.NewOptBool(true),
		UnsubscribeButton: siteapi.NewOptString(" "),
	}))
	cleared := read()
	assert.Empty(t, cleared.ImageUrl.Value, "a blank image removes it")
	assert.Empty(t, cleared.Buttons)
	assert.True(t, cleared.LinkPreview)
	assert.Empty(t, cleared.UnsubscribeButton.Value, "a blank label removes the button")

	badImage := edit(siteapi.TelegramBroadcastContentInput{ImageUrl: siteapi.NewOptString("http://example.com/a.png")})
	badButton := edit(siteapi.TelegramBroadcastContentInput{Buttons: buttonsOf(siteapi.TelegramButton{Label: "x", URL: "javascript:1"})})
	testhelper.SiteRefusal(t, badImage, http.StatusUnprocessableEntity, "an image is an https URL")
	testhelper.SiteRefusal(t, badButton, http.StatusUnprocessableEntity, "a button URL is http or https")
}

// Schedule applies the rule an Automation's Publish applies to a step: the text as written is
// 4096 long at most, 1024 with an image; a message with no text and no image has nothing to send.
func TestSiteTelegramBroadcastScheduleKnowsTheCaptionLimit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	bot := env.AcmeTelegram(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	scheduled := func(text, image string) error {
		in := siteapi.TelegramBroadcastContentInput{Text: siteapi.NewOptString(text)}
		if image != "" {
			in.ImageUrl = siteapi.NewOptString(image)
		}
		b, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
			Name: "News", IntegrationId: siteapi.NewOptNilEntityId(idStr(int64(bot.Integration.ID))),
			Content: testhelper.SiteTelegramContent(in),
		}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		_, err = c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
			siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
		return err
	}
	const image = "https://cdn.example.com/a.png"

	require.NoError(t, scheduled(strings.Repeat("a", 4096), ""))
	testhelper.SiteRefusal(t, scheduled(strings.Repeat("a", 1025), image), http.StatusUnprocessableEntity, "a caption is 1024 at most")
	assert.NoError(t, scheduled(strings.Repeat("a", 1024), image))
	assert.NoError(t, scheduled("", image), "an image alone is a message")
	testhelper.SiteRefusal(t, scheduled(" ", ""), http.StatusUnprocessableEntity, "no text and no image")
}
