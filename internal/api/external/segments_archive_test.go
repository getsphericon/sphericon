package external_test

import (
	"context"
	"testing"

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

// /api archives, lists and restores a Segment; a draft or scheduled Broadcast refuses the
// archive with 409 naming it.
func TestExternalSegmentsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:write", "segments:read")
	ctx := context.Background()

	busy, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentProPlanID)})
	require.NoError(t, err)
	refused, ok := busy.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", busy)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindBroadcast, refused.UsedBy[0].Kind)
	assert.Equal(t, fixtures.BroadcastProSegmentName, refused.UsedBy[0].Name)

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
