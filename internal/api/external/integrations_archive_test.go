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

func listedIntegrationIDs(t *testing.T, c *externalapi.Client, archived bool) []externalapi.EntityId {
	t.Helper()
	res, err := c.IntegrationsList(context.Background(), externalapi.IntegrationsListParams{
		PageSize: externalapi.NewOptInt32(100),
		Archived: externalapi.NewOptBool(archived),
	})
	require.NoError(t, err)
	list, ok := res.(*externalapi.IntegrationsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(i externalapi.IntegrationResource, _ int) externalapi.EntityId { return i.ID })
}

// /api archives, lists and restores an Integration (ADR 0049). The default one is refused with a
// 409 while live work uses it; archiving clears the default flag and restoring does not return it.
func TestExternalIntegrationsArchiveAndRestore(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()

	busy, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: entityIDString(fixtures.IntegrationAcmeDefaultID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, busy)

	id := entityIDString(fixtures.IntegrationAcmeSesID)
	out, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: id})
	require.NoError(t, err)
	archived, ok := out.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", out)
	assert.True(t, archived.ArchivedAt.IsSet())
	assert.False(t, archived.IsDefault)
	assert.NotContains(t, listedIntegrationIDs(t, c, false), id)
	shown := listedIntegrationIDs(t, c, true)
	assert.Contains(t, shown, id)
	assert.Contains(t, shown, entityIDString(fixtures.IntegrationAcmeArchivedID))

	got, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationResource{}, got, "an archived Integration is still readable by id")

	back, err := c.IntegrationsUnarchive(ctx, externalapi.IntegrationsUnarchiveParams{ID: id})
	require.NoError(t, err)
	restored, ok := back.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", back)
	assert.False(t, restored.ArchivedAt.IsSet())
	assert.Contains(t, listedIntegrationIDs(t, c, false), id)

	missing, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: entityIDString(fixtures.IntegrationGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsArchiveNotFound{}, missing)
}

// usersOfRefusal are the (kind, id) pairs an archive refusal names.
func usersOfRefusal(t *testing.T, got externalapi.IntegrationsArchiveRes) []externalapi.UsedBy {
	t.Helper()
	refused, ok := got.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", got)
	return refused.UsedBy
}

func automationUsers(users []externalapi.UsedBy, id int64) []externalapi.UsedBy {
	return lo.Filter(users, func(u externalapi.UsedBy, _ int) bool {
		return u.Kind == externalapi.UsedByKindAutomation && u.ID == entityIDString(id)
	})
}

// An active Automation with an email step uses the Integration it names, so that Integration is
// refused an archive while it is live.
func TestExternalIntegrationArchiveIsRefusedWhileAnAutomationNamesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()
	archive := func(id int64) externalapi.IntegrationsArchiveRes {
		out, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: entityIDString(id)})
		require.NoError(t, err)
		return out
	}
	env.DB.AutomationVersion.UpdateOneID(fixtures.AutomationJobsGreetingVersionID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(ctx)

	users := usersOfRefusal(t, archive(fixtures.IntegrationAcmeMailgunID))

	assert.Len(t, automationUsers(users, fixtures.AutomationJobsGreetingID), 1)
}

// An Automation that names an Integration no longer counts as a user of the default one.
func TestExternalDefaultIntegrationIsNotUsedByAnAutomationThatNamesAnother(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()
	archiveDefault := func() []externalapi.UsedBy {
		out, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: entityIDString(fixtures.IntegrationAcmeDefaultID)})
		require.NoError(t, err)
		return usersOfRefusal(t, out)
	}
	require.Len(t, automationUsers(archiveDefault(), fixtures.AutomationJobsGreetingID), 1, "naming none, it uses the default")

	env.DB.AutomationVersion.UpdateOneID(fixtures.AutomationJobsGreetingVersionID).SetIntegrationID(fixtures.IntegrationAcmeMailgunID).ExecX(ctx)

	assert.Empty(t, automationUsers(archiveDefault(), fixtures.AutomationJobsGreetingID))
}

func TestExternalIntegrationsArchiveNeedsTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	reader := env.ExternalScoped(t, "integrations:read")
	id := entityIDString(fixtures.IntegrationAcmeSesID)

	archive, err := reader.IntegrationsArchive(context.Background(), externalapi.IntegrationsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsArchiveForbidden{}, archive)
	unarchive, err := reader.IntegrationsUnarchive(context.Background(), externalapi.IntegrationsUnarchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUnarchiveForbidden{}, unarchive)
}
