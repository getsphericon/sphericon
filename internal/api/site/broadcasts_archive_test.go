package site_test

import (
	"context"
	"testing"

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

// A failed broadcast is history too; a draft, a scheduled and a sending one are not archived.
func TestSiteBroadcastsArchiveOnlySentOrFailed(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	failed, err := c.SiteBroadcastsArchive(ctx, archiveParams(fixtures.BroadcastFailedID))
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastResource{}, failed)

	for _, id := range []int64{fixtures.BroadcastDraftID, fixtures.BroadcastScheduledID, fixtures.BroadcastSendingID} {
		out, err := c.SiteBroadcastsArchive(ctx, archiveParams(id))
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteBroadcastsArchiveUnprocessableEntity{}, out, "broadcast %d", id)
	}

	missing, err := c.SiteBroadcastsArchive(ctx, archiveParams(999999))
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsArchiveNotFound{}, missing)
}

// Deleting removes a draft for good; a sent broadcast is archived, never deleted.
func TestSiteBroadcastsDeleteOnlyPurgesDrafts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	del := func(id int64) (siteapi.SiteBroadcastsDeleteRes, error) {
		return c.SiteBroadcastsDelete(ctx, siteapi.SiteBroadcastsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(id)})
	}

	gone, err := del(fixtures.BroadcastDraftID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsDeleteNoContent{}, gone)
	draftGone, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsGetNotFound{}, draftGone, "a draft is purged, not archived")

	sent, err := del(fixtures.BroadcastSentID)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsDeleteUnprocessableEntity{}, sent)
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

// Archive and restore are recorded in the Audit log with their actor.
func TestSiteBroadcastsArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteBroadcastsArchive(ctx, archiveParams(fixtures.BroadcastSentID))
	require.NoError(t, err)
	_, err = c.SiteBroadcastsUnarchive(ctx, siteapi.SiteBroadcastsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastSentID})
	require.NoError(t, err)

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"broadcast.discard", "broadcast.restore"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, "Python crash course is here", entries[0].TargetName)
}
