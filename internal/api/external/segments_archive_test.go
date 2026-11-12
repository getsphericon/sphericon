package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listedSegmentIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	list, err := c.SegmentsList(context.Background(), externalapi.SegmentsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	return lo.Map(list.Items, func(s externalapi.SegmentResource, _ int) externalapi.EntityId { return s.ID })
}

// /api archives, lists and restores a Segment.
func TestExternalSegmentsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeSegmentsWrite, apitokens.ScopeSegmentsRead)
	ctx := context.Background()

	id := entityIDString(fixtures.SegmentRetiredID)
	archived, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, listedSegmentIDs(t, c, false), id)
	assert.Contains(t, listedSegmentIDs(t, c, true), id)

	_, err = c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
	require.NoError(t, err)

	restored, err := c.SegmentsUnarchive(ctx, externalapi.SegmentsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedSegmentIDs(t, c, false), id)
}

// A draft is not Live use (ADR 0099): /api archives a Segment only a draft names, and scheduling
// that draft is then refused with 422 naming the Segment.
func TestExternalSegmentOnlyADraftNamesIsArchivedAndTheDraftIsNotScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeSegmentsWrite, apitokens.ScopeBroadcastsSend)
	ctx := context.Background()

	archived, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentProPlanID)})
	require.NoError(t, err)
	_, err = c.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(entityIDString(fixtures.BroadcastProSegmentID)))), ScheduledAt: externalapi.Timestamp(time.Now().Add(time.Hour))},
		externalapi.BroadcastsScheduleParams{ID: entityIDString(fixtures.BroadcastProSegmentID)})

	assert.True(t, archived.ArchivedAt.IsSet())
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, fixtures.SegmentProPlanName)
}

// An archived Segment is read-only to /api until it is unarchived (ADR 0100).
func TestExternalArchivedSegmentIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeSegmentsWrite)

	_, err := c.SegmentsUpdate(context.Background(), &externalapi.UpdateSegmentInput{Revision: externalapi.Revision(env.SegmentRevision(t.Context(), t, fixtures.SegmentArchivedID)), Name: externalapi.NewOptName("Renamed")},
		externalapi.SegmentsUpdateParams{ID: entityIDString(fixtures.SegmentArchivedID)})

	testhelper.ExternalRefusal(t, err, http.StatusConflict)
}
