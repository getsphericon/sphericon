package external_test

import (
	"context"
	"net/http"
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
	list, err := c.IntegrationsList(context.Background(), externalapi.IntegrationsListParams{PageSize: externalapi.NewOptInt32(100)})
	require.NoError(t, err)
	return lo.Map(list.Items, func(i externalapi.IntegrationResource, _ int) externalapi.EntityId { return i.ID })
}

// /api deletes an Integration for good (ADR 0049: it is discarded). The default one is refused
// with a 409 while live work uses it; a deleted one leaves the list, gives up the default and is
// still readable by id.
func TestExternalIntegrationsDelete(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()

	err := c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationAcmeDefaultID)})
	busy := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	assert.NotEmpty(t, busy.UsedBy)

	id := entityIDString(fixtures.IntegrationAcmeSesID)
	err = c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: id})
	require.NoError(t, err)
	listed := listedIntegrationIDs(t, c)
	assert.NotContains(t, listed, id)
	assert.NotContains(t, listed, entityIDString(fixtures.IntegrationAcmeArchivedID))

	deleted, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	assert.True(t, deleted.DiscardedAt.IsSet())
	assert.False(t, deleted.IsDefault)

	err = c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

// A deleted Integration is read, never changed (ADR 0100): an edit and a new hook key answer 404,
// and Get still answers.
func TestExternalIntegrationsDeletedOneIsReadOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "integrations:read", "integrations:write")
	ctx := context.Background()
	id := entityIDString(fixtures.IntegrationAcmeArchivedID)

	_, err := c.IntegrationsUpdate(ctx, &externalapi.UpdateIntegrationInput{Name: externalapi.NewOptString("Renamed")},
		externalapi.IntegrationsUpdateParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	_, err = c.IntegrationsRegenerateKey(ctx, externalapi.IntegrationsRegenerateKeyParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	row, err := c.IntegrationsGet(ctx, externalapi.IntegrationsGetParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, "Old relay (archived)", row.Name, "the refused edit left it as it was")
}

// usersOfRefusal are the (kind, id) pairs a delete refusal names.
func usersOfRefusal(t *testing.T, err error) []externalapi.UsedBy {
	t.Helper()
	return testhelper.ExternalRefusal(t, err, http.StatusConflict).UsedBy
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
	del := func(id int64) error {
		return c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: entityIDString(id)})
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

	err := c.IntegrationsDelete(t.Context(), externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationMeridianDefaultID)})
	users := usersOfRefusal(t, err)

	assert.Len(t, automationUsers(users, fixtures.AutomationMeridianWelcomeID), 1, "naming none, it uses the default")
	assert.Empty(t, automationUsers(users, fixtures.AutomationMeridianViaRelayID))
}

func TestExternalIntegrationsDeleteNeedsTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	reader := env.ExternalScoped(t, "integrations:read")

	err := reader.IntegrationsDelete(context.Background(), externalapi.IntegrationsDeleteParams{ID: entityIDString(fixtures.IntegrationAcmeSesID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}
