package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixtures: Liam has a chat of his own; Emma has none of her own but her phone (a Visitor of
// hers) does, and a Binding of a Visitor reaches its Contact.
func TestContactBindingsAreItsOwnAndItsVisitors(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	list := func(contactID int64) []siteapi.SiteBindingResource {
		res, err := owner.SiteTelegramListBindings(t.Context(), siteapi.SiteTelegramListBindingsParams{
			WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(contactID),
		})
		require.NoError(t, err)
		bound, ok := res.(*siteapi.SiteTelegramListBindingsOKApplicationJSON)
		require.Truef(t, ok, "got %T", res)
		return *bound
	}

	liams := list(fixtures.ContactLiamID)
	emmas := list(101)
	noones := list(fixtures.ContactBobID)

	require.Len(t, liams, 1)
	assert.Equal(t, "acme_fixture_bot", liams[0].BotUsername.Or(""))
	assert.Equal(t, "Acme bot", liams[0].IntegrationName)
	assert.Len(t, emmas, 1)
	assert.Empty(t, noones)
}

func TestTheSiteIssuesAStartLinkForAContact(t *testing.T) {
	env := testhelper.Setup(t)
	bot := env.AcmeTelegram(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	id := bot.Integration.ID

	res, err := owner.SiteTelegramIssueStartLink(t.Context(),
		&siteapi.IssueTelegramStartLinkInput{IntegrationId: siteapi.NewOptNilEntityId(id)},
		siteapi.SiteTelegramIssueStartLinkParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.EntityId(fixtures.ContactBobID)})

	require.NoError(t, err)
	link, ok := res.(*siteapi.TelegramStartLinkResource)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, link.URL, "https://t.me/"+testhelper.AcmeTelegramUsername+"?start=")
}
