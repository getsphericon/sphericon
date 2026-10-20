package external_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func listedAutomationIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	res, err := c.AutomationsList(context.Background(), externalapi.AutomationsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*externalapi.AutomationsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(a externalapi.AutomationResource, _ int) externalapi.EntityId { return a.ID })
}

// /api archives, lists and restores an Automation; active Enrollments refuse the archive with 409
// and their count, and an Automation with Enrollments is not deleted.
func TestExternalAutomationsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:write", "automations:read")
	ctx := context.Background()

	busy, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	refused, ok := busy.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", busy)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindEnrollment, refused.UsedBy[0].Kind)
	assert.EqualValues(t, 2, refused.UsedBy[0].Count.Or(0))

	id := entityIDString(fixtures.AutomationNotifySlackID)
	out, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: id})
	require.NoError(t, err)
	archived, ok := out.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, listedAutomationIDs(t, c, false), id)
	assert.Contains(t, listedAutomationIDs(t, c, true), id)

	del, err := c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: id})
	require.NoError(t, err)
	refusedDelete, ok := del.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", del)
	require.Len(t, refusedDelete.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindEnrollment, refusedDelete.UsedBy[0].Kind)

	back, err := c.AutomationsUnarchive(ctx, externalapi.AutomationsUnarchiveParams{ID: id})
	require.NoError(t, err)
	restored, ok := back.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedAutomationIDs(t, c, false), id)
}

// Contact erasure reaches the Enrollments of an archived Automation.
func TestExternalContactsEraseRemovesEnrollmentsOfAnArchivedAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	eraseErasable(t, env)

	exists, err := env.DB.AutomationRun.Query().Where(automationrun.ID(fixtures.AutomationRunArchivedErasableID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}
