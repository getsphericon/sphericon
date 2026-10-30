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

func listedBroadcastIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	res, err := c.BroadcastsList(context.Background(), externalapi.BroadcastsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*externalapi.BroadcastsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(b externalapi.BroadcastResource, _ int) externalapi.EntityId { return b.ID })
}

// A sent broadcast is archived and restored through /api; the list hides it by default and
// shows it with archived=true; a draft is not archivable and a scheduled one is refused.
func TestExternalBroadcastsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, authorScopes...)
	ctx := context.Background()

	out, err := c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: sentBroadcast})
	require.NoError(t, err)
	archived, ok := out.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())

	assert.NotContains(t, listedBroadcastIDs(t, c, false), sentBroadcast)
	assert.Contains(t, listedBroadcastIDs(t, c, true), sentBroadcast)

	report, err := c.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: sentBroadcast})
	require.NoError(t, err)
	sr, ok := report.(*externalapi.BroadcastReport)
	require.Truef(t, ok, "an archived broadcast keeps its report, got %T", report)
	assert.EqualValues(t, 11, sr.RecipientsTotal)

	back, err := c.BroadcastsUnarchive(ctx, externalapi.BroadcastsUnarchiveParams{ID: sentBroadcast})
	require.NoError(t, err)
	restored, ok := back.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedBroadcastIDs(t, c, false), sentBroadcast)

	draft, err := c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: draftBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsArchiveUnprocessableEntity{}, draft)
	sched, err := c.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: schedBroadcast})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsArchiveUnprocessableEntity{}, sched)
}

func TestExternalBroadcastsArchiveNeedsTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	readOnly := env.ExternalScoped(t, "broadcasts:read")
	ctx := context.Background()
	id := entityIDString(fixtures.BroadcastSentID)

	arch, err := readOnly.BroadcastsArchive(ctx, externalapi.BroadcastsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsArchiveForbidden{}, arch)
	unarch, err := readOnly.BroadcastsUnarchive(ctx, externalapi.BroadcastsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUnarchiveForbidden{}, unarch)
}
