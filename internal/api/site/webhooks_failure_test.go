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

	failing, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{Slug: fixtures.AcmeSlug, ID: siteapi.EntityId(fixtures.WebhookCodebasicsFailingID)})
	require.NoError(t, err)
	res, ok := failing.(*siteapi.SiteWebhookEndpointResource)
	require.Truef(t, ok, "got %T", failing)
	f, has := res.Failure.Get()
	require.True(t, has)
	assert.Equal(t, siteapi.SiteWebhookErrorKindHTTPStatus, f.Kind)
	assert.Equal(t, int32(503), f.Status.Value)
	assert.Equal(t, int32(7), f.ConsecutiveFailures)
	assert.True(t, time.Time(f.Since).Before(time.Time(f.LastFailureAt)))

	healthy, err := c.SiteWebhooksGet(ctx, siteapi.SiteWebhooksGetParams{Slug: fixtures.AcmeSlug, ID: siteapi.EntityId(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.False(t, healthy.(*siteapi.SiteWebhookEndpointResource).Failure.IsSet())
}
