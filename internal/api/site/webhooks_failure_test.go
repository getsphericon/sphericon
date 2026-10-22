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

func TestSiteWebhooksShowTheFailureState(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	failing, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.WebhookCodebasicsFailingID)})
	require.NoError(t, err)
	res, ok := failing.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", failing)
	f, has := res.Failure.Get()
	require.True(t, has)
	assert.Equal(t, siteapi.SiteWebhookErrorKindHTTPStatus, f.Kind)
	assert.Equal(t, int32(503), f.Status.Value)
	assert.Equal(t, int32(7), f.ConsecutiveFailures)
	assert.True(t, time.Time(f.Since).Before(time.Time(f.LastFailureAt)))

	healthy, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.False(t, healthy.(*siteapi.SiteWebhookEndpointResource).Failure.IsSet())
}

func TestSiteWebhooksShowWhyAnEndpointTurnedItselfOff(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	gone, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.WebhookCodebasicsGoneID)})
	require.NoError(t, err)
	res, ok := gone.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", gone)
	assert.False(t, res.Enabled)
	assert.Equal(t, siteapi.SiteWebhookDisabledReasonGone, res.DisabledReason.Value)
	assert.True(t, res.DisabledAt.IsSet(), "when it turned off is shown")

	byOwner, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.WebhookCodebasicsDisabledID)})
	require.NoError(t, err)
	ownerRes, ok := byOwner.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", byOwner)
	assert.Equal(t, siteapi.SiteWebhookDisabledReasonOwner, ownerRes.DisabledReason.Value)
	assert.True(t, ownerRes.DisabledAt.IsSet())

	healthy, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.False(t, healthy.(*siteapi.SiteWebhookEndpointResource).DisabledReason.IsSet())
	assert.False(t, healthy.(*siteapi.SiteWebhookEndpointResource).DisabledAt.IsSet())
}
