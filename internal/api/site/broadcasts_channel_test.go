package site_test

import (
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Broadcast takes the Channel of the content it is created with (ADR 0080), and shows it on
// every read: its own field and its content's arm.
func TestSiteBroadcastHasTheChannelOfItsContent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Launch",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hello")}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res := created
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	read := got

	assert.Equal(t, siteapi.BroadcastChannelEmail, read.Channel)
	assert.Equal(t, siteapi.EmailBroadcastContentBroadcastContentSum, read.Content.OneOf.Type)
	assert.Equal(t, "Hello", testhelper.SiteEmailOf(read).Subject)
}

// A Broadcast names its Channel when it is created: a create without content, or with content of
// a Channel the contract does not list, is refused and stores nothing.
func TestSiteBroadcastCreateWithoutAChannelIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before := env.DB.Broadcast.Query().CountX(ctx)
	in := &siteapi.CreateBroadcastInput{Name: "No channel", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{})}
	params := siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	noContent := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		delete(body, "content")
	}))
	_, missing := noContent.SiteBroadcastsCreate(ctx, in, params)
	noChannel := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		delete(body["content"].(map[string]any), "channel")
	}))
	_, unnamed := noChannel.SiteBroadcastsCreate(ctx, in, params)
	after := env.DB.Broadcast.Query().CountX(ctx)

	testhelper.SiteRefusal(t, missing, http.StatusBadRequest)
	testhelper.SiteRefusal(t, unnamed, http.StatusBadRequest)
	assert.Equal(t, before, after, "a refused create stores nothing")
}

// A Broadcast sends only through an Integration of its Channel (ADR 0074): one of another Channel
// is refused on create and on update, and a refused update changes nothing.
func TestSiteBroadcastRefusesAnIntegrationOfAnotherChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	sms := siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeSmsID))
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	_, created := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:          "Through SMS",
		Content:       testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		IntegrationId: sms,
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	_, updated := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(idStr(fixtures.BroadcastDraftID)))), IntegrationId: sms},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	refusedCreate := testhelper.SiteRefusal(t, created, http.StatusUnprocessableEntity)
	assert.Contains(t, refusedCreate.Detail.Value, "another channel")
	testhelper.SiteRefusal(t, updated, http.StatusUnprocessableEntity)
	assert.Equal(t, before.IntegrationID, after.IntegrationID, "a refused update changes nothing")
}

// The Channel is fixed at creation: an update whose content is of another Channel is refused, and
// the Broadcast keeps its Channel and content.
func TestSiteBroadcastUpdateCannotChangeTheChannel(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(idStr(fixtures.BroadcastDraftID)))),
		Content: testhelper.SiteTelegramEdit("Elsewhere"),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, before.Channel, after.Channel)
	assert.Equal(t, before.Subject, after.Subject)
}

// The list filters by Channel: email lists the email Broadcasts, telegram the Telegram one, sms
// the SMS ones, and together they are all of them.
func TestSiteBroadcastsListFiltersByChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	list := func(channel siteapi.OptBroadcastChannel) *siteapi.SiteBroadcastsListOK {
		t.Helper()
		got, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID, Channel: channel})
		require.NoError(t, err)
		page := got
		return page
	}

	everyList := list(siteapi.OptBroadcastChannel{})
	emailList := list(siteapi.NewOptBroadcastChannel(siteapi.BroadcastChannelEmail))
	telegramList := list(siteapi.NewOptBroadcastChannel(siteapi.BroadcastChannelTelegram))
	smsList := list(siteapi.NewOptBroadcastChannel(siteapi.BroadcastChannelSMS))
	channels := lo.Uniq(lo.Map(emailList.Items, func(b siteapi.SiteBroadcastResource, _ int) siteapi.BroadcastChannel { return b.Channel }))

	assert.Equal(t, []siteapi.BroadcastChannel{siteapi.BroadcastChannelEmail}, channels)
	assert.Equal(t, []string{fixtures.BroadcastTelegramDraftName}, lo.Map(telegramList.Items, func(b siteapi.SiteBroadcastResource, _ int) string { return b.Name }))
	assert.ElementsMatch(t, []string{fixtures.BroadcastSmsDraftName, fixtures.BroadcastSmsBlankDraftName, fixtures.BroadcastSmsSentName},
		lo.Map(smsList.Items, func(b siteapi.SiteBroadcastResource, _ int) string { return b.Name }))
	assert.Equal(t, everyList.TotalItems, emailList.TotalItems+telegramList.TotalItems+smsList.TotalItems)
}
