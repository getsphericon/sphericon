package site_test

import (
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

	msg, ok := out.(*siteapi.SiteCapturedMessageResource)
	require.Truef(t, ok, "got %T", out)
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

	page := out.(*siteapi.SiteCapturedMessagesListOK)
	require.NotEmpty(t, page.Items)
	assert.Equal(t, fixtures.CapturedMessageGlobexWelcomeSubject, page.Items[0].Subject)
}

func TestSiteCapturedMessageOfAnotherWorkspaceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	read, err := john.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	require.NoError(t, err)
	del, err := john.SiteCapturedMessagesDelete(t.Context(), siteapi.SiteCapturedMessagesDeleteParams{
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	require.NoError(t, err)
	foreign, err := john.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteCapturedMessagesGetNotFound{}, read)
	assert.IsType(t, &siteapi.SiteCapturedMessagesDeleteNotFound{}, del)
	assert.IsType(t, &siteapi.SiteCapturedMessagesGetNotFound{}, foreign, "a non-member gets nothing")
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	still, err := jane.SiteCapturedMessagesGet(t.Context(), siteapi.SiteCapturedMessagesGetParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteCapturedMessageResource{}, still, "the message survived")
}

func TestSiteCapturedMessageIsDeletedByAnyMember(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	params := siteapi.SiteCapturedMessagesDeleteParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.CapturedMessageGlobexWelcomeID),
	}

	out, err := mary.SiteCapturedMessagesDelete(t.Context(), params)
	require.NoError(t, err)
	again, err := mary.SiteCapturedMessagesDelete(t.Context(), params)
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteCapturedMessagesDeleteNoContent{}, out)
	assert.IsType(t, &siteapi.SiteCapturedMessagesDeleteNotFound{}, again)
	list, err := mary.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	page := list.(*siteapi.SiteCapturedMessagesListOK)
	require.Len(t, page.Items, 1)
	assert.Equal(t, int64(fixtures.CapturedMessageGlobexNewsletterID), int64(page.Items[0].ID), "the other one stays")
}

func TestSiteCapturedMessagesAreAllDeletedForTheOwnWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := mary.SiteCapturedMessagesDeleteAll(t.Context(), siteapi.SiteCapturedMessagesDeleteAllParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	denied, err := john.SiteCapturedMessagesDeleteAll(t.Context(), siteapi.SiteCapturedMessagesDeleteAllParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteCapturedMessagesDeleteAllNoContent{}, out)
	assert.IsType(t, &siteapi.ProblemDetails{}, denied)
	assert.Equal(t, 404, int(denied.(*siteapi.ProblemDetails).Status.Value))
	own, err := mary.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.Empty(t, own.(*siteapi.SiteCapturedMessagesListOK).Items)
	other, err := john.SiteCapturedMessagesList(t.Context(), siteapi.SiteCapturedMessagesListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Len(t, other.(*siteapi.SiteCapturedMessagesListOK).Items, 1, "Acme's message is untouched")
}
