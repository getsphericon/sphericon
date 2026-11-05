package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

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

	_, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Broken", Body: siteapi.NewOptString("<p>{% if %}</p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "render body")

	_, err = c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Broken",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString("<div><span>hi</div>")}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	refusedBroadcast := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refusedBroadcast.Detail.Or(""), "<span> is never closed")

	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("{% if %}")}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: fixtures.BroadcastDraftID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Broken", TriggerEvent: "contact.created",
		Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString("<p>hi</p></div>")},
		},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	refusedAutomation := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refusedAutomation.Detail.Or(""), "</div> closes nothing")
}
