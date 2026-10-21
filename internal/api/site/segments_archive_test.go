package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listSegments(t *testing.T, c *siteapi.Client, archived bool) []int64 {
	t.Helper()
	res, err := c.SiteSegmentsList(context.Background(), siteapi.SiteSegmentsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteSegmentsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(s siteapi.SiteSegmentResource, _ int) int64 { return int64(s.ID) })
}

func archiveSegment(c *siteapi.Client, id int64) (siteapi.SiteSegmentsArchiveRes, error) {
	return c.SiteSegmentsArchive(context.Background(), siteapi.SiteSegmentsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
}

func unarchiveSegment(c *siteapi.Client, id int64) (siteapi.SiteSegmentsUnarchiveRes, error) {
	return c.SiteSegmentsUnarchive(context.Background(), siteapi.SiteSegmentsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
}

// A Segment that a draft or scheduled Broadcast uses cannot be archived: the 409 names those
// Broadcasts (ADR 0049), and the Segment stays in the list.
func TestSiteSegmentArchiveRefusedWhileLiveBroadcastUsesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := acme.Broadcast().UpdateOneID(fixtures.BroadcastScheduledID).SetSegmentID(fixtures.SegmentProPlanID).Save(ctx)
	require.NoError(t, err)

	got, err := archiveSegment(c, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	refused, ok := got.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", got)

	names := lo.Map(refused.UsedBy, func(u siteapi.UsedBy, _ int) string { return u.Name })
	assert.ElementsMatch(t, []string{fixtures.BroadcastProSegmentName, fixtures.BroadcastScheduledName}, names)
	kinds := lo.Map(refused.UsedBy, func(u siteapi.UsedBy, _ int) siteapi.UsedByKind { return u.Kind })
	assert.Equal(t, []siteapi.UsedByKind{siteapi.UsedByKindBroadcast, siteapi.UsedByKindBroadcast}, kinds)
	assert.EqualValues(t, http.StatusConflict, refused.Status.Or(0))

	assert.Contains(t, listSegments(t, c, false), int64(fixtures.SegmentProPlanID))
}

// A Segment that only a failed or sent Broadcast used is archived; the Broadcast keeps its
// reference, the Segment leaves the default list, shows with archived=true, can still be
// opened, and comes back when restored.
func TestSiteSegmentArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SegmentRetiredID)

	out, err := archiveSegment(c, id)
	require.NoError(t, err)
	archived, ok := out.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listSegments(t, c, false), id)
	shown := listSegments(t, c, true)
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.SegmentArchivedID))
	assert.NotContains(t, shown, int64(fixtures.SegmentActiveID))

	got, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err)
	assert.IsTypef(t, &siteapi.SiteSegmentResource{}, got, "an archived segment can still be opened")

	failed, err := env.DB.Scoped(fixtures.AcmeID).Broadcast().Get(ctx, fixtures.BroadcastFailedID)
	require.NoError(t, err)
	assert.Equal(t, id, lo.FromPtr(failed.SegmentID))

	back, err := unarchiveSegment(c, id)
	require.NoError(t, err)
	restored, ok := back.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listSegments(t, c, false), id)
}

// A sent Broadcast's report still names the Segment it was sent to after that Segment is archived.
func TestSiteSentBroadcastStillNamesItsArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	b, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastArchivedID)})
	require.NoError(t, err)
	broadcast, ok := b.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", b)
	segmentID, ok := broadcast.SegmentId.Get()
	require.True(t, ok)
	assert.EqualValues(t, fixtures.SegmentArchivedID, segmentID)

	s, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: segmentID})
	require.NoError(t, err)
	seg, ok := s.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", s)
	assert.Equal(t, fixtures.SegmentArchivedName, seg.Name)
}

// An archive does not hold the name: a new Segment may take an archived one's name.
func TestSiteSegmentNameIsFreeAfterArchive(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteSegmentsCreate(context.Background(), &siteapi.SiteCreateSegmentInput{
		Name:       fixtures.SegmentArchivedName,
		Definition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentResource{}, out)
}

// Archive and restore are recorded in the Audit log with their actor.
func TestSiteSegmentArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := archiveSegment(c, fixtures.SegmentRetiredID)
	require.NoError(t, err)
	_, err = unarchiveSegment(c, fixtures.SegmentRetiredID)
	require.NoError(t, err)

	entries := lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"segment.discard", "segment.restore"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, fixtures.SegmentRetiredName, entries[0].TargetName)
}
