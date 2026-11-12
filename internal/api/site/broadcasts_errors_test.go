package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every broadcast operation is workspace-scoped: a foreign workspaceID is a 404, a
// non-numeric id a 400, an unknown or foreign-workspace id a 404.
func TestSiteBroadcastsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	draft := idStr(fixtures.BroadcastDraftID)
	missing := idStr(99999)
	// Every Schedule below is refused before its revision is read: foreign, missing or sending.
	when := siteapi.ScheduleBroadcastInput{Revision: 1, ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))}
	testSend := siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("preview@example.com")}

	t.Run("foreign workspace", func(t *testing.T) {
		_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "x", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{})}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: foreign})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(draft)))}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsArchive(ctx, siteapi.SiteBroadcastsArchiveParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(draft)))}, siteapi.SiteBroadcastsSendParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsTestSend(ctx, &testSend, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: foreign, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
	})

	t.Run("unknown id", func(t *testing.T) {
		_, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(missing)))}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsArchive(ctx, siteapi.SiteBroadcastsArchiveParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(missing)))}, siteapi.SiteBroadcastsSendParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsTestSend(ctx, &testSend, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: acme, ID: missing})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)

		// A broadcast of another workspace is invisible too, and stays kept.
		_, err = c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastGlobexID)})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteBroadcastsArchive(ctx, siteapi.SiteBroadcastsArchiveParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastGlobexID)})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		kept, err := env.DB.Broadcast.Query().Where(broadcast.ID(fixtures.BroadcastGlobexID), broadcast.ArchivedAtIsNil()).Exist(ctx)
		require.NoError(t, err)
		assert.True(t, kept)
	})

	t.Run("invalid segment and integration references are 422", func(t *testing.T) {
		bad := siteapi.NewOptNilEntityId(99999999) // no such row in any workspace
		_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "x", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}), SegmentId: bad}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "x", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}), IntegrationId: bad}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(draft))), SegmentId: bad}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(draft))), IntegrationId: bad}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: draft})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		count, err := env.DB.Broadcast.Query().Count(ctx)
		require.NoError(t, err)
		_, err = c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{Name: "ok", Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{})}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)

		after, err := env.DB.Broadcast.Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, count+1, after, "rejected creates persisted nothing")
	})

	t.Run("state machine conflicts are 422", func(t *testing.T) {
		sending := idStr(fixtures.BroadcastSendingID)
		_, err := c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: acme, ID: sending})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(idStr(fixtures.BroadcastSentID))))}, siteapi.SiteBroadcastsSendParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastSentID)})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		stored, err := env.DB.Broadcast.Get(ctx, fixtures.BroadcastSendingID)
		require.NoError(t, err)
		assert.Equal(t, broadcast.StatusSending, stored.Status)
	})
}

func TestSiteBroadcastsListPaginatesAndRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{
		WorkspaceId: fixtures.AcmePublicID, Page: siteapi.NewOptInt32(2), PageSize: siteapi.NewOptInt32(2),
	})
	require.NoError(t, err)
	page := res
	assert.Len(t, page.Items, 2)
	assert.EqualValues(t, 2, page.Page)
	assert.Greater(t, page.TotalItems, int32(4))
	assert.Greater(t, page.TotalPages, int32(2))

	_, err = c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.GlobexPublicID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	_, err = env.SiteAnonymous(t).SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteBroadcastsTestSendDeliversPreview(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteBroadcastsTestSend(ctx, &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("preview@example.com")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "preview@example.com", msgs[0].To)
}
