package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteCapturedMessageIsReadInFullByAnyMember(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	out, err := mary.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	require.NoError(t, err)

	msg := out
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, msg.Subject)
	assert.Contains(t, msg.HTML, "start here")
	assert.Equal(t, "Welcome, start here: https://globex.test/start", msg.Text)
	assert.Equal(t, []string{"https://globex.test/start"}, msg.Links)
	require.Len(t, msg.Headers, 3)
	assert.Equal(t, "From", msg.Headers[0].Name)
}

func TestSiteCapturedMessagesListLeavesTheContentOut(t *testing.T) {
	env := testhelper.Setup(t)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	out, err := jane.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	page := out
	require.NotEmpty(t, page.Items)
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, page.Items[0].Subject)
}

func TestSiteCapturedMessageOfAnotherWorkspaceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, read := john.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	del := john.SiteCapturedMessagesDelete(t.Context(), siteapi.SiteCapturedMessagesDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	_, foreign := john.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})

	testhelper.SiteRefusal(t, read, http.StatusNotFound)
	testhelper.SiteRefusal(t, del, http.StatusNotFound)
	testhelper.SiteRefusal(t, foreign, http.StatusNotFound, "a non-member gets nothing")
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := jane.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	assert.NoError(t, err, "the message survived")
}

func TestSiteCapturedMessageIsDeletedByAnyMember(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	params := siteapi.SiteCapturedMessagesDeleteParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	}

	out := mary.SiteCapturedMessagesDelete(t.Context(), params)
	again := mary.SiteCapturedMessagesDelete(t.Context(), params)

	require.NoError(t, out)
	testhelper.SiteRefusal(t, again, http.StatusNotFound)
	page, err := mary.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexNewsletterID), int64(page.Items[0].ID), "the other one stays")
}

func TestSiteCapturedMessagesAreAllDeletedForTheOwnWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out := mary.SiteCapturedMessagesDeleteAll(t.Context(), siteapi.SiteCapturedMessagesDeleteAllParams{WorkspaceId: fixtures.GlobexPublicID})
	denied := john.SiteCapturedMessagesDeleteAll(t.Context(), siteapi.SiteCapturedMessagesDeleteAllParams{WorkspaceId: fixtures.GlobexPublicID})

	require.NoError(t, out)
	testhelper.SiteRefusal(t, denied, http.StatusNotFound)
	own, err := mary.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.Empty(t, own.Items)
	other, err := john.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Len(t, other.Items, 1, "Acme's message is untouched")
}
