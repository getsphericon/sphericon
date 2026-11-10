package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// siteEmailStepVia is an email step through the Integration id names; an unset one is the default.
func siteEmailStepVia(id siteapi.OptNilEntityId) siteapi.SiteAutomationStep {
	return siteapi.SiteAutomationStep{
		Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Hi"),
		Body: siteapi.NewOptString("<p>Hi</p>"), IntegrationId: id,
	}
}

// Each email step names the Integration it sends through (ADR 0081): it is saved with the step and
// read back on it, a step that names none sends through the default, and steps rewritten without
// it go back to the default.
func TestSiteAutomationStepsNameTheirIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	mailgun := siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationAcmeMailgunID))

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Onboarding", TriggerEvent: "signup",
		Steps: []siteapi.SiteAutomationStep{siteEmailStepVia(mailgun), siteEmailStepVia(siteapi.OptNilEntityId{})},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	params := siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID}
	renamed, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Name: siteapi.NewOptName("Renamed")}, params)
	require.NoError(t, err)
	kept := renamed
	rewritten, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Steps: []siteapi.SiteAutomationStep{siteEmailStepVia(siteapi.OptNilEntityId{})},
	}, params)
	require.NoError(t, err)
	back := rewritten

	require.Len(t, res.Draft.Value.Steps, 2)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, res.Draft.Value.Steps[0].IntegrationId.Or(0), "the first step through the relay")
	assert.Empty(t, res.Draft.Value.Steps[1].IntegrationId.Or(0), "the second step through the default")
	assert.Equal(t, res.Draft.Value.Steps, kept.Draft.Value.Steps, "an update that leaves the steps out keeps them")
	require.Len(t, back.Draft.Value.Steps, 1)
	assert.Empty(t, back.Draft.Value.Steps[0].IntegrationId.Or(0), "a step written without one goes back to the default")
}

// A step's Integration that does not exist in the Workspace is a 422 and stores nothing.
func TestSiteAutomationsRefuseAStepsUnknownIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	steps := []siteapi.SiteAutomationStep{siteEmailStepVia(siteapi.NewOptNilEntityId(idStr(fixtures.IntegrationGlobexID)))} // another Workspace's
	draftBefore := env.DB.AutomationVersion.GetX(ctx, fixtures.AutomationWelcomeSeriesDraftVersionID).Steps

	_, create := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "x", TriggerEvent: "signup", Steps: steps},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	_, update := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Steps: steps},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: idStr(fixtures.AutomationWelcomeSeriesID)})

	testhelper.SiteRefusal(t, create, http.StatusUnprocessableEntity)
	testhelper.SiteRefusal(t, update, http.StatusUnprocessableEntity)
	assert.False(t, env.DB.Automation.Query().Where(automation.Name("x")).ExistX(ctx), "a refused create stores nothing")
	assert.Equal(t, draftBefore, env.DB.AutomationVersion.GetX(ctx, fixtures.AutomationWelcomeSeriesDraftVersionID).Steps, "a refused update changes nothing")
}
