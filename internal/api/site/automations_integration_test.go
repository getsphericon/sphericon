package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation names the Integration its email steps send through: it is set on create, moved
// or cleared with a JSON Merge Patch (an explicit null clears, an absent key keeps), and read back.
func TestSiteAutomationsNameTheirIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Onboarding", TriggerEvent: "signup", IntegrationId: siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeMailgunID)),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", created)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, res.IntegrationId.Or(0))

	params := siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID}
	renamed, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Name: siteapi.NewOptString("Renamed")}, params)
	require.NoError(t, err)
	kept, ok := renamed.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", renamed)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, kept.IntegrationId.Or(0), "an absent key keeps it")

	var null siteapi.OptNilEntityId
	null.SetToNull()
	cleared, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{IntegrationId: null}, params)
	require.NoError(t, err)
	got, ok := cleared.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", cleared)
	assert.Empty(t, got.IntegrationId.Or(0), "an explicit null goes back to the default")
}

// An Integration that does not exist in the Workspace is a 422 and stores nothing.
func TestSiteAutomationsRefuseAnUnknownIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	bad := siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationGlobexID)) // another Workspace's

	create, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "signup", IntegrationId: bad},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsCreateUnprocessableEntity{}, create)

	update, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{IntegrationId: bad},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: idStr(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsUpdateUnprocessableEntity{}, update)

	assert.False(t, env.DB.Automation.Query().Where(automation.Name("x")).ExistX(ctx), "a refused create stores nothing")
	assert.Nil(t, env.DB.Automation.GetX(ctx, fixtures.AutomationWelcomeSeriesID).IntegrationID, "a refused update changes nothing")
}
