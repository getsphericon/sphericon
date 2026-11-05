package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func archiveParams(id int64) siteapi.SiteBroadcastsArchiveParams {
	return siteapi.SiteBroadcastsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)}
}

func listBroadcasts(t *testing.T, c *siteapi.Client, archived bool) []siteapi.SiteBroadcastResource {
	t.Helper()
	res, err := c.SiteBroadcastsList(context.Background(), siteapi.SiteBroadcastsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteBroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	return list.Items
}

func broadcastIDs(items []siteapi.SiteBroadcastResource) []int64 {
	return lo.Map(items, func(b siteapi.SiteBroadcastResource, _ int) int64 { return int64(b.ID) })
}

// A sent broadcast is archived: it leaves the default list, is listed by archived=true,
// keeps its figures, and comes back when restored.
func TestSiteBroadcastsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.BroadcastSentID)
	before := lo.FindOrElse(listBroadcasts(t, c, false), siteapi.SiteBroadcastResource{}, func(b siteapi.SiteBroadcastResource) bool { return int64(b.ID) == id })

	out, err := c.SiteBroadcastsArchive(ctx, archiveParams(id))
	require.NoError(t, err)
	archived, ok := out.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.Equal(t, before.Stats, archived.Stats, "archiving leaves the figures alone")

	kept := broadcastIDs(listBroadcasts(t, c, false))
	assert.NotContains(t, kept, id)
	shown := broadcastIDs(listBroadcasts(t, c, true))
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.BroadcastArchivedID))
	assert.NotContains(t, shown, int64(fixtures.BroadcastDraftID))

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	opened, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "an archived broadcast can still be opened, got %T", got)
	assert.Equal(t, before.Stats, opened.Stats)

	back, err := c.SiteBroadcastsUnarchive(ctx, siteapi.SiteBroadcastsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	restored, ok := back.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, broadcastIDs(listBroadcasts(t, c, false)), id)
}

// A draft and a failed broadcast are archived too; a scheduled and a sending one are work in
// flight and are refused with 409.
func TestSiteBroadcastsArchiveRefusesWorkInFlight(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	for _, id := range []int64{fixtures.BroadcastDraftID, fixtures.BroadcastFailedID} {
		out, err := c.SiteBroadcastsArchive(ctx, archiveParams(id))
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastResource{}, out, "broadcast %d", id)
	}

	for _, id := range []int64{fixtures.BroadcastScheduledID, fixtures.BroadcastSendingID} {
		out, err := c.SiteBroadcastsArchive(ctx, archiveParams(id))
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsArchiveConflict{}, out, "broadcast %d", id)
	}

	missing, err := c.SiteBroadcastsArchive(ctx, archiveParams(999999))
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsArchiveNotFound{}, missing)
}

// A draft is archived, not deleted: it leaves the list for the archived view and comes back a
// draft when unarchived.
func TestSiteADraftBroadcastIsArchivedAndUnarchived(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.BroadcastDraftID)

	out, err := c.SiteBroadcastsArchive(ctx, archiveParams(id))
	require.NoError(t, err)
	archived, ok := out.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, broadcastIDs(listBroadcasts(t, c, false)), id)
	assert.Contains(t, broadcastIDs(listBroadcasts(t, c, true)), id)

	back, err := c.SiteBroadcastsUnarchive(ctx, siteapi.SiteBroadcastsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	require.NoError(t, err)
	restored, ok := back.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Equal(t, siteapi.SiteBroadcastStatusDraft, restored.Status)
	assert.Contains(t, broadcastIDs(listBroadcasts(t, c, false)), id)
}

// An archived draft is read-only until it is unarchived: an edit, a schedule and a send answer
// 409 and leave it as it was.
func TestSiteAnArchivedDraftBroadcastIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := siteapi.EntityId(fixtures.BroadcastDraftID)
	_, err := c.SiteBroadcastsArchive(ctx, archiveParams(fixtures.BroadcastDraftID))
	require.NoError(t, err)

	upd, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Name: siteapi.NewOptString("Renamed")},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateConflict{}, upd)

	sched, err := c.SiteBroadcastsSchedule(ctx, &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
		siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsScheduleConflict{}, sched)

	send, err := c.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsSendConflict{}, send)

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	kept, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, fixtures.BroadcastDraftName, kept.Name)
	assert.Equal(t, siteapi.SiteBroadcastStatusDraft, kept.Status)
}

// The dashboard counts an archived broadcast: archiving moves no figure.
func TestSiteAnalyticsIgnoresArchiving(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteAnalyticsOverviewParams{
		WorkspaceId: fixtures.AcmePublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange90d),
	}
	read := func() *siteapi.SiteAnalyticsOverview {
		res, err := c.SiteAnalyticsOverview(ctx, params)
		require.NoError(t, err)
		ov, ok := res.(*siteapi.SiteAnalyticsOverview)
		require.Truef(t, ok, "got %T", res)
		return ov
	}
	before := read()

	_, err := c.SiteBroadcastsArchive(ctx, archiveParams(fixtures.BroadcastSentID))
	require.NoError(t, err)

	after := read()
	assert.Equal(t, before, after)
}

// Archive and unarchive are recorded in the Audit log with their actor.
func TestSiteBroadcastsArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteBroadcastsArchive(ctx, archiveParams(fixtures.BroadcastSentID))
	require.NoError(t, err)
	_, err = c.SiteBroadcastsUnarchive(ctx, siteapi.SiteBroadcastsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastSentID})
	require.NoError(t, err)

	entries := lo.Map(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"broadcast.archive", "broadcast.unarchive"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, "Python crash course is here", entries[0].TargetName)
}
