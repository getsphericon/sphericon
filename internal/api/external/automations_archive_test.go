package external_test

import (
	"context"
	"net/http"
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
	list, err := c.AutomationsList(context.Background(), externalapi.AutomationsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	return lo.Map(list.Items, func(a externalapi.AutomationResource, _ int) externalapi.EntityId { return a.ID })
}

// /api archives, lists and unarchives an Automation; active Enrollments refuse the archive with 409
// and their count, and an archived Automation takes no edit.
func TestExternalAutomationsArchiveAndUnarchive(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:write", "automations:read")
	ctx := context.Background()

	_, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	refused := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindEnrollment, refused.UsedBy[0].Kind)
	assert.EqualValues(t, 2, refused.UsedBy[0].Count.Or(0))

	id := entityIDString(fixtures.AutomationNotifySlackID)
	archived, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.NotContains(t, listedAutomationIDs(t, c, false), id)
	assert.Contains(t, listedAutomationIDs(t, c, true), id)

	_, err = c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(id))), Name: externalapi.NewOptName("Renamed")}, externalapi.AutomationsUpdateParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusConflict, "an archived automation is read-only")

	restored, err := c.AutomationsUnarchive(ctx, externalapi.AutomationsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedAutomationIDs(t, c, false), id)
}

// An Automation that has sent mail is archived like any other, never deleted (ADR 0100): it leaves
// the list, shows in the archived one and is still read by id.
func TestExternalAutomationsArchiveKeepsOneThatSentMail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:write", "automations:read")
	ctx := context.Background()
	id := entityIDString(fixtures.AutomationOnOpenID)

	_, err := c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: id})
	require.NoError(t, err)
	_, err = c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
	require.NoError(t, err, "an archived automation is still read by id")

	assert.NotContains(t, listedAutomationIDs(t, c, false), id)
	assert.Contains(t, listedAutomationIDs(t, c, true), id)
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
