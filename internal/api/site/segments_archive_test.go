package site_test

import (
	"context"
	"net/http"
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

func listSegments(t *testing.T, c *siteapi.Client, archived bool) []int64 {
	t.Helper()
	list, err := c.SiteSegmentsList(context.Background(), siteapi.SiteSegmentsListParams{
		WorkspaceId: fixtures.AcmePublicID,
		PageSize:    siteapi.NewOptInt32(100),
		Archived:    siteapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	return lo.Map(list.Items, func(s siteapi.SiteSegmentResource, _ int) int64 { return int64(s.ID) })
}

func archiveSegment(c *siteapi.Client, id int64) (*siteapi.SiteSegmentResource, error) {
	return c.SiteSegmentsArchive(context.Background(), siteapi.SiteSegmentsArchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
}

func unarchiveSegment(c *siteapi.Client, id int64) (*siteapi.SiteSegmentResource, error) {
	return c.SiteSegmentsUnarchive(context.Background(), siteapi.SiteSegmentsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
}

// A draft is not Live use (ADR 0099, ADR 0100): a Segment only a draft Broadcast names is
// archived, and scheduling that draft is then refused, naming the archived Segment; the draft stays
// a draft.
func TestSiteSegmentOnlyADraftNamesIsArchivedAndTheDraftIsNotScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	archived, err := archiveSegment(c, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())

	_, err = c.SiteBroadcastsSchedule(t.Context(), &siteapi.ScheduleBroadcastInput{ScheduledAt: siteapi.Timestamp(time.Now().Add(time.Hour))},
		siteapi.SiteBroadcastsScheduleParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastProSegmentID)})
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, fixtures.SegmentProPlanName)

	draft, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastProSegmentID)})
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteBroadcastStatusDraft, draft.Status)
}

// An archived Segment is read-only until it is unarchived (ADR 0100): an edit answers 409 and
// changes nothing; after Unarchive the same edit is saved.
func TestSiteArchivedSegmentIsReadOnlyUntilUnarchived(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	rename := func() (*siteapi.SiteSegmentResource, error) {
		return c.SiteSegmentsUpdate(t.Context(), &siteapi.SiteUpdateSegmentInput{Name: siteapi.NewOptString("Renamed")},
			siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentArchivedID)})
	}

	_, refused := rename()
	kept, err := c.SiteSegmentsGet(t.Context(), siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentArchivedID)})
	require.NoError(t, err)
	_, err = unarchiveSegment(c, fixtures.SegmentArchivedID)
	require.NoError(t, err)
	renamed, err := rename()
	require.NoError(t, err)

	conflict := testhelper.SiteRefusal(t, refused, http.StatusConflict)
	assert.EqualValues(t, http.StatusConflict, conflict.Status.Or(0))
	assert.Equal(t, fixtures.SegmentArchivedName, kept.Name)
	assert.Equal(t, "Renamed", renamed.Name)
}

// A scheduled Broadcast holds a copy of its Segment's Condition (ADR 0065), so its Segment is
// archived, and the Broadcast still names it as where its audience came from.
func TestSiteSegmentOfAScheduledBroadcastIsArchived(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	archived, err := archiveSegment(c, fixtures.SegmentRenewalAudienceID)
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())

	b, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastScheduledRenewalID)})
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteBroadcastStatusScheduled, b.Status)
	assert.Equal(t, idStr(fixtures.SegmentRenewalAudienceID), b.SegmentId.Value)
}

// A Segment that only a failed or sent Broadcast used is archived; the Broadcast keeps its
// reference, the Segment leaves the default list, shows with archived=true, can still be
// opened, and comes back when restored.
func TestSiteSegmentArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	id := int64(fixtures.SegmentRetiredID)

	archived, err := archiveSegment(c, id)
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listSegments(t, c, false), id)
	shown := listSegments(t, c, true)
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, int64(fixtures.SegmentArchivedID))
	assert.NotContains(t, shown, int64(fixtures.SegmentActiveID))

	_, err = c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(id)})
	require.NoError(t, err, "an archived segment can still be opened")

	failed, err := env.DB.Scoped(fixtures.AcmeID).Broadcast().Get(ctx, fixtures.BroadcastFailedID)
	require.NoError(t, err)
	assert.Equal(t, id, lo.FromPtr(failed.SegmentID))

	restored, err := unarchiveSegment(c, id)
	require.NoError(t, err)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listSegments(t, c, false), id)
}

// A sent Broadcast's report still names the Segment it was sent to after that Segment is archived.
func TestSiteSentBroadcastStillNamesItsArchivedSegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	broadcast, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastArchivedID)})
	require.NoError(t, err)
	segmentID, ok := broadcast.SegmentId.Get()
	require.True(t, ok)
	assert.EqualValues(t, fixtures.SegmentArchivedID, segmentID)

	seg, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: segmentID})
	require.NoError(t, err)
	assert.Equal(t, fixtures.SegmentArchivedName, seg.Name)
}

// An archive does not hold the name: a new Segment may take an archived one's name.
func TestSiteSegmentNameIsFreeAfterArchive(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSegmentsCreate(context.Background(), &siteapi.SiteCreateSegmentInput{
		Name:      fixtures.SegmentArchivedName,
		Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
}

// Archive and Unarchive are recorded in the Audit log with their actor, as the owner names them
// (ADR 0100).
func TestSiteSegmentArchiveIsAudited(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := archiveSegment(c, fixtures.SegmentRetiredID)
	require.NoError(t, err)
	_, err = unarchiveSegment(c, fixtures.SegmentRetiredID)
	require.NoError(t, err)

	entries := lo.Map(env.QueuedEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"segment.archive", "segment.unarchive"}, actions)
	assert.Equal(t, events.ActorUser, entries[0].Actor.Kind)
	assert.Equal(t, fixtures.SegmentRetiredName, entries[0].TargetName)
}
