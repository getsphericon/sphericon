package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Two renames of a Custom field made on one revision: the second is a 409 revision_changed, and the
// field keeps the first's name (ADR 0119).
func TestSiteCustomFieldUpdateOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	field := fieldByKey(t, c, fixtures.AcmePublicID, "plan")

	first, err := updateField(c, fixtures.AcmePublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Name: siteapi.NewOptName("First")})
	require.NoError(t, err)
	_, err = updateField(c, fixtures.AcmePublicID, field.ID, &siteapi.SiteUpdateCustomFieldInput{Revision: field.Revision, Name: siteapi.NewOptName("Second")})

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.CustomField.GetX(context.Background(), int64(field.ID))
	assert.Equal(t, "First", stored.Name)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// Two saves of a Webhook made on one revision: the second is a 409 revision_changed, and the endpoint
// keeps the first's URL.
func TestSiteWebhookUpdateOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteWebhooksUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)}
	read, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams(params))
	require.NoError(t, err)

	first, err := c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{Revision: read.Revision, URL: siteapi.NewOptString("https://hooks.example.com/first")}, params)
	require.NoError(t, err)
	_, err = c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{Revision: read.Revision, URL: siteapi.NewOptString("https://hooks.example.com/second")}, params)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.WebhookEndpoint.GetX(ctx, fixtures.WebhookCodebasicsID)
	assert.Equal(t, "https://hooks.example.com/first", stored.URL)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// A delivery that records a failure while the Webhook's editor is open writes Bookkeeping columns
// alone: the save goes through. One that turns the endpoint off changes what the editor sends
// (enabled): the save, which would turn it back on, is refused, and the endpoint stays off.
func TestSiteWebhookSaveSurvivesAFailureRecordButNotATurnOff(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteWebhooksUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)}
	read, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams(params))
	require.NoError(t, err)

	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).AddConsecutiveFailures(1).SetLastFailureAt(time.Now()).ExecX(ctx)
	saved, err := c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{Revision: read.Revision, URL: siteapi.NewOptString("https://hooks.example.com/kept")}, params)
	require.NoError(t, err, "a failure record alone leaves the revision")

	env.DB.WebhookEndpoint.UpdateOneID(fixtures.WebhookCodebasicsID).
		SetDisabledAt(time.Now()).SetDisabledReason(webhookendpoint.DisabledReasonPersistentFailure).ExecX(ctx)
	_, err = c.SiteWebhooksUpdate(ctx, &siteapi.UpdateWebhookInput{Revision: saved.Revision, Enabled: siteapi.NewOptBool(true)}, params)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	assert.NotNil(t, env.DB.WebhookEndpoint.GetX(ctx, fixtures.WebhookCodebasicsID).DisabledAt, "the endpoint stays off")
}

// Two saves of the Brand kit made on one revision: the second is a 409 revision_changed, and the kit
// keeps the first's values (ADR 0119).
func TestSiteBrandKitSaveOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	ctx := context.Background()
	read := getBrandKit(t, c, fixtures.InitechPublicID)
	first, second := brandKitInput(), brandKitInput()
	first.Revision, first.AccentColor = read.Revision, "#111111"
	second.Revision, second.AccentColor = read.Revision, "#222222"

	saved, err := c.SiteBrandKitSave(ctx, first, siteapi.SiteBrandKitSaveParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	_, err = c.SiteBrandKitSave(ctx, second, siteapi.SiteBrandKitSaveParams{WorkspaceId: fixtures.InitechPublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.BrandKit.GetX(ctx, fixtures.BrandKitInitechID)
	assert.Equal(t, "#111111", stored.AccentColor)
	assert.EqualValues(t, saved.Revision, stored.Revision)
}
