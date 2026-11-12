package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listedBroadcastIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	list, err := c.BroadcastsList(context.Background(), externalapi.BroadcastsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	return lo.Map(list.Items, func(b externalapi.BroadcastResource, _ int) externalapi.EntityId { return b.ID })
}

// A sent broadcast is archived and restored through /api; the list hides it by default and
// shows it with archived=true. A draft is archived too; a scheduled one is refused with 409.
func TestExternalBroadcastsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	archived, err := c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listedBroadcastIDs(t, c, false), sentBroadcast)
	assert.Contains(t, listedBroadcastIDs(t, c, true), sentBroadcast)

	sr, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.EqualValues(t, 11, sr.RecipientsTotal)

	restored, err := c.BroadcastsUnarchive(ctx, externalapi.BroadcastsUnarchiveParams{ID: sentBroadcast})
	require.NoError(t, err)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedBroadcastIDs(t, c, false), sentBroadcast)

	_, err = c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: draftBroadcast})
	require.NoError(t, err)
	_, err = c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: schedBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusConflict)
}

// An archived draft is read-only through /api until it is unarchived: an edit and a change of
// its Segment answer 409.
func TestExternalAnArchivedDraftBroadcastIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()
	_, err := c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: draftBroadcast})
	require.NoError(t, err)

	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), Name: externalapi.NewOptName("Renamed")},
		externalapi.BroadcastsUpdateParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusConflict)

	_, err = c.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(draftBroadcast))), SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: draftBroadcast})
	testhelper.ExternalRefusal(t, err, http.StatusConflict)
}

func TestExternalBroadcastsArchiveNeedsTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	readOnly := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead)
	ctx := context.Background()
	id := entityIDString(fixtures.BroadcastSentID)

	_, err := readOnly.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = readOnly.BroadcastsUnarchive(ctx, externalapi.BroadcastsUnarchiveParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}
