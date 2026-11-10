package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Acme owner fixture user owns the Acme workspace. Broadcasts live under
// /w/{workspaceID}/broadcasts and start life as drafts.
func TestSiteBroadcastsCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Spring sale",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Big news"), Body: siteapi.NewOptString("<p>Hi {{ first_name }}</p>")}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	assert.Equal(t, "Spring sale", res.Name)
	assert.Equal(t, "Big news", testhelper.SiteEmailOf(res).Subject)
	assert.Equal(t, siteapi.SiteBroadcastStatusDraft, res.Status)
	assert.Equal(t, int32(0), res.Stats.RecipientsTotal)

	// Fetch the created broadcast back by id (selection by key).
	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	gotRes := got
	assert.Equal(t, res.ID, gotRes.ID)

	// Update renames a draft.
	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Name: siteapi.NewOptName("Spring sale (v2)"),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	updRes := updated
	assert.Equal(t, "Spring sale (v2)", updRes.Name)

	// Schedule moves it to scheduled with a future time.
	when := time.Now().Add(24 * time.Hour)
	sched, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{
		ScheduledAt: siteapi.Timestamp(when),
	}, siteapi.SiteBroadcastsScheduleParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	schedRes := sched
	assert.Equal(t, siteapi.SiteBroadcastStatusScheduled, schedRes.Status)
	assert.True(t, schedRes.ScheduledAt.Set)

	// Send moves it into the sending state.
	sent, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	sentRes := sent
	assert.Equal(t, siteapi.SiteBroadcastStatusSending, sentRes.Status)

	// Sending an already-sending broadcast is rejected with 422.
	_, err = c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: workspaceID, ID: res.ID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	// Editing a non-draft broadcast is rejected with 422.
	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Name: siteapi.NewOptName("nope"),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	testhelper.SiteRefusal(t, err, http.StatusConflict)

	// A sent broadcast is history: Archive keeps it (it stays addressable).
	env.DeliverQueuedBroadcasts(t)
	_, err = c.SiteBroadcastsArchive(ctx, siteapi.SiteBroadcastsArchiveParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)

	_, err = c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
}

// Broadcasts are scoped to the workspace: a workspaceID the user does not own is a 404.
func TestSiteBroadcastsRequireOwnedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteBroadcastsList(ctx, siteapi.SiteBroadcastsListParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// JSON Merge Patch on update: explicit null clears sender name, sender email, the
// audience segment and the integration; absent keys keep their values. Fixture draft broadcast
// 100 has a sender; segment 1 is an anchor segment.
func TestSiteBroadcastsUpdateNullClearsAbsentKeeps(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: 100}

	// Absent keys keep values: set the segment, the sender stays.
	res, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		SegmentId:     siteapi.NewOptNilEntityId(1),
		IntegrationId: siteapi.NewOptNilEntityId(1),
	}, params)
	require.NoError(t, err)
	got := res
	assert.EqualValues(t, 1, got.SegmentId.Or(0))
	assert.EqualValues(t, 1, got.IntegrationId.Or(0))
	assert.Equal(t, "CodeBasics", testhelper.SiteEmailOf(got).FromName.Or(""))
	assert.Equal(t, "hello@codebasics.dev", string(testhelper.SiteEmailOf(got).FromEmail.Or("")))

	// Explicit nulls clear all three.
	var nullSeg, nullIntegration siteapi.OptNilEntityId
	nullSeg.SetToNull()
	nullIntegration.SetToNull()
	res, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content:       testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{FromName: nullString(), FromEmail: nullEmail()}),
		SegmentId:     nullSeg,
		IntegrationId: nullIntegration,
	}, params)
	require.NoError(t, err)
	got = res
	assert.Empty(t, testhelper.SiteEmailOf(got).FromName.Or(""))
	assert.Empty(t, testhelper.SiteEmailOf(got).FromEmail.Or(""))
	assert.Empty(t, got.SegmentId.Or(0))
	assert.Empty(t, got.IntegrationId.Or(0))
	assert.Equal(t, "Summer sale teaser", got.Name, "absent name kept")
}

// A segment or integration id of another Workspace is refused with a client error,
// never stored: the scoped client verifies the reference on create and on update.
func TestSiteBroadcastsCreateRefusesAForeignSegmentOrIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID}
	before := env.DB.Broadcast.Query().CountX(ctx)

	_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:      "forged",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentGlobexID)),
	}, workspaceID)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:          "forged",
		Content:       testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{}),
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationGlobexID)),
	}, workspaceID)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	assert.Equal(t, before, env.DB.Broadcast.Query().CountX(ctx), "a refused create stores nothing")
}

func TestSiteBroadcastsUpdateRefusesAForeignSegmentOrIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)}
	before := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		SegmentId: siteapi.NewOptNilEntityId(idStr(fixtures.SegmentGlobexID)),
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationGlobexID)),
	}, params)
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, before.SegmentID, after.SegmentID, "a refused update changes nothing")
	assert.Equal(t, before.IntegrationID, after.IntegrationID)
}
