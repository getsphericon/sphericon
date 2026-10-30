package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Content that cannot render is refused where it is saved, with the reason (ADR 0040).
func TestSiteRefusesContentThatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	tpl, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Broken", Body: siteapi.NewOptString("<p>{% if %}</p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	refused, ok := tpl.(*siteapi.SiteTemplatesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", tpl)
	assert.Contains(t, refused.Detail.Or(""), "render body")

	bc, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "Broken", Body: siteapi.NewOptString("<div><span>hi</div>"),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	refusedBroadcast, ok := bc.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", bc)
	assert.Contains(t, refusedBroadcast.Detail.Or(""), "<span> is never closed")

	upd, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		Subject: siteapi.NewOptString("{% if %}"),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, upd)

	auto, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Broken", TriggerEvent: "contact.created",
		Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString("<p>hi</p></div>")},
		},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	refusedAutomation, ok := auto.(*siteapi.SiteAutomationsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", auto)
	assert.Contains(t, refusedAutomation.Detail.Or(""), "</div> closes nothing")
}
