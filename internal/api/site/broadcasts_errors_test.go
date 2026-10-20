package site_test

import (
	"context"
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
	when := siteapi.SiteScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))}
	testSend := siteapi.SiteTestSendBroadcastInput{Email: "preview@example.com"}

	t.Run("foreign workspace", func(t *testing.T) {
		r1, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{Name: "x"}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: foreign})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsCreateNotFound{}, r1)
		r2, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsGetNotFound{}, r2)
		r3, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsUpdateNotFound{}, r3)
		r4, err := c.SiteBroadcastsDelete(ctx, siteapi.SiteBroadcastsDeleteParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsDeleteNotFound{}, r4)
		r5, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsSendNotFound{}, r5)
		r6, err := c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsScheduleNotFound{}, r6)
		r7, err := c.SiteBroadcastsTestSend(ctx, &testSend, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: foreign, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsTestSendNotFound{}, r7)
	})

	t.Run("unknown id", func(t *testing.T) {
		r1, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsGetNotFound{}, r1)
		r2, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsUpdateNotFound{}, r2)
		r3, err := c.SiteBroadcastsDelete(ctx, siteapi.SiteBroadcastsDeleteParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsDeleteNotFound{}, r3)
		r4, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsSendNotFound{}, r4)
		r5, err := c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsScheduleNotFound{}, r5)
		r6, err := c.SiteBroadcastsTestSend(ctx, &testSend, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: acme, ID: missing})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsTestSendNotFound{}, r6)

		// A broadcast of another workspace is invisible too, and survives the delete.
		r7, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastGlobexID)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsGetNotFound{}, r7)
		r8, err := c.SiteBroadcastsDelete(ctx, siteapi.SiteBroadcastsDeleteParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastGlobexID)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsDeleteNotFound{}, r8)
		exists, err := env.DB.Broadcast.Query().Where(broadcast.ID(fixtures.BroadcastGlobexID)).Exist(ctx)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("invalid segment and integration references are 422", func(t *testing.T) {
		bad := siteapi.NewOptNilEntityId(99999999) // no such row in any workspace
		r1, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{Name: "x", SegmentId: bad}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, r1)
		r2, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{Name: "x", IntegrationId: bad}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, r2)
		r3, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{SegmentId: bad}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, r3)
		r4, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{IntegrationId: bad}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: acme, ID: draft})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, r4)
		count, err := env.DB.Broadcast.Query().Count(ctx)
		require.NoError(t, err)
		again, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{Name: "ok"}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteBroadcastResource{}, again)
		after, err := env.DB.Broadcast.Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, count+1, after, "rejected creates persisted nothing")
	})

	t.Run("state machine conflicts are 422", func(t *testing.T) {
		sending := idStr(fixtures.BroadcastSendingID)
		r1, err := c.SiteBroadcastsSchedule(ctx, &when, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: acme, ID: sending})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsScheduleUnprocessableEntity{}, r1)
		r2, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: acme, ID: idStr(fixtures.BroadcastSentID)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsSendUnprocessableEntity{}, r2)
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
	page := res.(*siteapi.SiteBroadcastsListOK)
	assert.Len(t, page.Items, 2)
	assert.EqualValues(t, 2, page.Page)
	assert.Greater(t, page.TotalItems, int32(4))
	assert.Greater(t, page.TotalPages, int32(2))

	foreign, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsListNotFound{}, foreign)

	_, err = env.SiteAnonymous(t).SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteBroadcastsTestSendDeliversPreview(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteBroadcastsTestSend(ctx, &siteapi.SiteTestSendBroadcastInput{Email: "preview@example.com"},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)
	if !assert.IsType(t, &siteapi.SiteTestSendBroadcastResult{}, res) {
		return
	}
	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "preview@example.com", msgs[0].To)
}
