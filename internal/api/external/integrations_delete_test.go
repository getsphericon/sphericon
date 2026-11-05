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

func listedIntegrationIDs(t *testing.T, c *externalapi.Client) []externalapi.EntityId {
	t.Helper()
	res, err := c.IntegrationsList(context.Background(), externalapi.IntegrationsListParams{PageSize: externalapi.NewOptInt32(100)})
	require.NoError(t, err)
	list, ok := res.(*externalapi.IntegrationsListOK)
	require.Truef(t, ok, "got %T", res)
	return lo.Map(list.Items, func(i externalapi.IntegrationResource, _ int) externalapi.EntityId { return i.ID })
}

// /api deletes an Integration for good (ADR 0049: it is discarded). The default one is refused
// with a 409 while live work uses it; a deleted one leaves the list, gives up the default and is
// still readable by id.
func TestExternalIntegrationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()

	busy, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationAcmeDefaultID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, busy)

	id := entityIDString(fixtures.IntegrationAcmeSesID)
	out, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNoContent{}, out)
	listed := listedIntegrationIDs(t, c)
	assert.NotContains(t, listed, id)
	assert.NotContains(t, listed, entityIDString(fixtures.IntegrationAcmeArchivedID))

	got, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	deleted, ok := got.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "a deleted Integration is still readable by id, got %T", got)
	assert.True(t, deleted.DiscardedAt.IsSet())
	assert.False(t, deleted.IsDefault)

	missing, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteNotFound{}, missing)
}

// A deleted Integration is read, never changed (ADR 0100): an edit and a new hook key answer 404,
// and Get still answers.
func TestExternalIntegrationsDeletedOneIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()
	id := entityIDString(fixtures.IntegrationAcmeArchivedID)

	renamed, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Name: externalapi.NewOptString("Renamed")},
		externalapi.IntegrationsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsUpdateNotFound{}, renamed)

	rekeyed, err := c.IntegrationsRegenerateKey(ctx, externalapi.IntegrationsRegenerateKeyParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsRegenerateKeyNotFound{}, rekeyed)

	got, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	row, ok := got.(*externalapi.IntegrationResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, "Old relay (archived)", row.Name, "the refused edit left it as it was")
}

// usersOfRefusal are the (kind, id) pairs a delete refusal names.
func usersOfRefusal(t *testing.T, got externalapi.IntegrationsDeleteRes) []externalapi.UsedBy {
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

// An active Automation with an email step through an Integration uses it, so that Integration is
// refused a delete while it is live. Fixtures: the second email step of "Jobs emails then tag"
// sends through the Mailgun relay.
func TestExternalIntegrationDeleteIsRefusedWhileAnAutomationNamesIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()
	del := func(id int64) externalapi.IntegrationsDeleteRes {
		out, err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(id)})
		require.NoError(t, err)
		return out
	}

	users := usersOfRefusal(t, del(fixtures.IntegrationAcmeMailgunID))

	assert.Len(t, automationUsers(users, fixtures.AutomationJobsEmailsThenTagID), 1)
	assert.Empty(t, automationUsers(users, fixtures.AutomationJobsGreetingID), "its steps send through the default")
}

// An Automation whose every email step names another Integration is no user of the default one.
// Fixtures: Meridian's welcome names no Integration; its relay welcome names its relay in every step.
func TestExternalDefaultIntegrationIsNotUsedByAnAutomationThatNamesAnother(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.MeridianID, "integrations:read", "integrations:write"))

	out, err := c.IntegrationsDelete(t.Context(), externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationMeridianDefaultID)})
	require.NoError(t, err)
	users := usersOfRefusal(t, out)

	assert.Len(t, automationUsers(users, fixtures.AutomationMeridianWelcomeID), 1, "naming none, it uses the default")
	assert.Empty(t, automationUsers(users, fixtures.AutomationMeridianViaRelayID))
}

func TestExternalIntegrationsDeleteNeedsTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	reader := env.ExternalScoped(t, "integrations:read")

	del, err := reader.IntegrationsDelete(context.Background(), externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationAcmeSesID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.IntegrationsDeleteForbidden{}, del)
}
