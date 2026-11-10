package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteTemplatesCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	res, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name:    "Welcome",
		Subject: siteapi.NewOptString("Welcome {{ first_name }}"),
		Body:    siteapi.NewOptString("<p>Hi</p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.Equal(t, "Welcome", res.Name)

	gotRes, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, res.ID, gotRes.ID)

	updRes, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{
		Name: siteapi.NewOptName("Welcome v2"),
	}, siteapi.SiteTemplatesUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, "Welcome v2", updRes.Name)

	err = c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)

	_, err = c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: workspaceID, ID: res.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// A test-send that is held (here: a suspended Workspace) is rejected with 422.
func TestSiteBroadcastsTestSendHeldIs422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	env.SuspendWorkspace(ctx, fixtures.AcmeID, time.Now())

	_, err := c.SiteBroadcastsTestSend(ctx, &siteapi.TestSendBroadcastInput{
		Email: siteapi.NewOptEmailAddress("qa@test.dev"),
	}, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

// Mapping only: the blank-name rule itself is tested at the templates module.
func TestSiteTemplatesMapABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{Name: siteapi.NewOptName("")},
		siteapi.SiteTemplatesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TemplateWelcomeID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}

func TestSiteTemplatesCreateMapsABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteTemplatesCreate(context.Background(), &siteapi.CreateTemplateInput{Name: ""},
		siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}
