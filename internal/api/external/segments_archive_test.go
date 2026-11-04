package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listedSegmentIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	res, err := c.SegmentsList(context.Background(), externalapi.SegmentsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*externalapi.SegmentsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(s externalapi.SegmentResource, _ int) externalapi.EntityId { return s.ID })
}

// /api archives, lists and restores a Segment.
func TestExternalSegmentsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:write", "segments:read")
	ctx := context.Background()

	id := entityIDString(fixtures.SegmentRetiredID)
	out, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
	require.NoError(t, err)
	archived, ok := out.(*externalapi.SegmentResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, listedSegmentIDs(t, c, false), id)
	assert.Contains(t, listedSegmentIDs(t, c, true), id)

	got, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentResource{}, got)

	back, err := c.SegmentsUnarchive(ctx, externalapi.SegmentsUnarchiveParams{ID: id})
	require.NoError(t, err)
	restored, ok := back.(*externalapi.SegmentResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedSegmentIDs(t, c, false), id)
}

// A draft is not Live use (ADR 0099): /api archives a Segment only a draft names, and scheduling
// that draft is then refused with 422 naming the Segment.
func TestExternalSegmentOnlyADraftNamesIsArchivedAndTheDraftIsNotScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:write", "broadcasts:send")
	ctx := context.Background()

	out, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentProPlanID)})
	require.NoError(t, err)
	res, err := c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
		externalapi.BroadcastsScheduleParams{ID: entityIDString(fixtures.BroadcastProSegmentID)})
	require.NoError(t, err)

	archived, ok := out.(*externalapi.SegmentResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	refused, ok := res.(*externalapi.BroadcastsScheduleUnprocessableEntity)
	require.Truef(t, ok, "got %T", res)
	assert.Contains(t, refused.Detail.Value, fixtures.SegmentProPlanName)
}

// An archived Segment is read-only to /api until it is unarchived (ADR 0100).
func TestExternalArchivedSegmentIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:write")

	res, err := c.SegmentsUpdate(context.Background(), &externalapi.UpdateSegmentInput{Name: externalapi.NewOptString("Renamed")},
		externalapi.SegmentsUpdateParams{ID: entityIDString(fixtures.SegmentArchivedID)})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.SegmentsUpdateConflict{}, res)
}
