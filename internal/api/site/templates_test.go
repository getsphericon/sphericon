package site_test

import (
	"context"
	"testing"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteTemplatesCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name:    "Welcome",
		Subject: siteapi.NewOptString("Welcome {{ first_name }}"),
		Body:    siteapi.NewOptString("<p>Hi</p>"),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, "Welcome", res.Name)

	got, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)

	updated, err := c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{
		Name: siteapi.NewOptString("Welcome v2"),
	}, siteapi.SiteTemplatesUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	updRes, ok := updated.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Equal(t, "Welcome v2", updRes.Name)

	del, err := c.SiteTemplatesDelete(ctx, siteapi.SiteTemplatesDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesDeleteNoContent{}, del)

	gone, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesGetNotFound{}, gone)
}

// A test-send that is held (here: a suspended Workspace) is rejected with 422.
func TestSiteBroadcastsTestSendHeldIs422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "Preview me", Subject: siteapi.NewOptString("Hi"),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	b := created.(*siteapi.SiteBroadcastResource)
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSuspendedAt(time.Now()).ExecX(ctx)

	out, err := c.SiteBroadcastsTestSend(ctx, &siteapi.SiteTestSendBroadcastInput{
		Email: "qa@test.dev",
	}, siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: b.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsTestSendUnprocessableEntity{}, out)
}

// Mapping only: the blank-name rule itself is tested at the templates module.
func TestSiteTemplatesMapABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	upd, err := c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{Name: siteapi.NewOptString("")},
		siteapi.SiteTemplatesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.TemplateWelcomeID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesUpdateUnprocessableEntity{}, upd)
	assert.Equal(t, fixtures.TemplateWelcomeName, env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID).Name)
}

func TestSiteTemplatesCreateMapsABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(context.Background(), &siteapi.SiteCreateEmailTemplateInput{Name: ""},
		siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesCreateUnprocessableEntity{}, created)
}
