package external_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Over the API a Broadcast is created on the Channel of its content and shows it on read; a create
// without a Channel is refused.
func TestExternalBroadcastIsCreatedOnTheChannelOfItsContent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	in := &externalapi.CreateBroadcastInput{
		Name:    "Launch",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("Hello")}),
	}
	c := env.ExternalScoped(t, authorScopes...)
	noChannel := env.ExternalVia(t, env.ScopedBearer(t, authorScopes...), testhelper.EditBody(func(body map[string]any) {
		delete(body, "content")
	}))

	created, err := c.BroadcastsCreate(ctx, in)
	require.NoError(t, err)
	refused, err := noChannel.BroadcastsCreate(ctx, in)
	require.NoError(t, err)

	b, ok := created.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, externalapi.BroadcastChannelEmail, b.Channel)
	assert.Equal(t, "Hello", testhelper.ExternalEmailOf(b).Subject)
	assert.IsType(t, &externalapi.BroadcastsCreateBadRequest{}, refused)
}

// An Integration of another Channel is refused over the API too.
func TestExternalBroadcastRefusesAnIntegrationOfAnotherChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)

	got, err := c.BroadcastsCreate(t.Context(), &externalapi.CreateBroadcastInput{
		Name:          "Through SMS",
		Content:       testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{}),
		IntegrationId: externalapi.NewOptNilEntityId(entityIDString(fixtures.IntegrationAcmeSmsID)),
	})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.BroadcastsCreateUnprocessableEntity{}, got)
}

// The API's list filters by Channel: email lists the email Broadcasts, telegram the Telegram one,
// sms the SMS ones, and together they are all of them.
func TestExternalBroadcastsListFiltersByChannel(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := t.Context()
	list := func(channel externalapi.OptBroadcastChannel) *externalapi.BroadcastsListOK {
		t.Helper()
		got, err := c.BroadcastsList(ctx, externalapi.BroadcastsListParams{Channel: channel})
		require.NoError(t, err)
		page, ok := got.(*externalapi.BroadcastsListOK)
		require.Truef(t, ok, "got %T", got)
		return page
	}

	everyList := list(externalapi.OptBroadcastChannel{})
	emailList := list(externalapi.NewOptBroadcastChannel(externalapi.BroadcastChannelEmail))
	telegramList := list(externalapi.NewOptBroadcastChannel(externalapi.BroadcastChannelTelegram))
	smsList := list(externalapi.NewOptBroadcastChannel(externalapi.BroadcastChannelSMS))
	channels := lo.Uniq(lo.Map(emailList.Items, func(b externalapi.BroadcastResource, _ int) externalapi.BroadcastChannel { return b.Channel }))

	assert.Equal(t, []externalapi.BroadcastChannel{externalapi.BroadcastChannelEmail}, channels)
	assert.Equal(t, []string{fixtures.BroadcastTelegramDraftName}, lo.Map(telegramList.Items, func(b externalapi.BroadcastResource, _ int) string { return b.Name }))
	assert.ElementsMatch(t, []string{fixtures.BroadcastSmsDraftName, fixtures.BroadcastSmsBlankDraftName},
		lo.Map(smsList.Items, func(b externalapi.BroadcastResource, _ int) string { return b.Name }))
	assert.Equal(t, everyList.TotalItems, emailList.TotalItems+telegramList.TotalItems+smsList.TotalItems)
}
