package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// /api names an Automation's Integration like /site does: set on create, kept when absent on
// update, cleared by an explicit null, refused with a 422 when it is not the Workspace's.
func TestExternalAutomationsNameTheirIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")
	ctx := context.Background()

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Onboarding", TriggerEvent: "signup", IntegrationId: externalapi.NewOptNilEntityId(entityIDString(fixtures.IntegrationAcmeMailgunID)),
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", created)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, res.IntegrationId.Or(0))

	renamed, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Name: externalapi.NewOptString("Renamed")},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	kept, ok := renamed.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", renamed)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, kept.IntegrationId.Or(0), "an absent key keeps it")

	var null externalapi.OptNilEntityId
	null.SetToNull()
	cleared, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{IntegrationId: null}, externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	got, ok := cleared.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", cleared)
	assert.Empty(t, got.IntegrationId.Or(0), "an explicit null goes back to the default")

	bad := externalapi.NewOptNilEntityId(entityIDString(fixtures.IntegrationGlobexID)) // another Workspace's
	refusedCreate, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "x", TriggerEvent: "signup", IntegrationId: bad})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsCreateUnprocessableEntity{}, refusedCreate)
	refusedUpdate, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{IntegrationId: bad}, externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsUpdateUnprocessableEntity{}, refusedUpdate)
}
