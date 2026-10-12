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

// audit.entry is selectable on a Webhook endpoint only with an Enterprise license
// (ADR 0022); without one the selection is rejected on create and update.
func TestWebhookEndpointSelectsAuditEntryOnlyUnderLicense(t *testing.T) {
	ctx := context.Background()
	create := func(env *testhelper.TestEnv) siteapi.SiteWebhooksCreateRes {
		res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksCreate(ctx,
			&siteapi.SiteCreateWebhookEndpointInput{URL: "https://x.test/audit", EventTypes: []string{"audit.entry"}},
			siteapi.SiteWebhooksCreateParams{Slug: fixtures.AcmeSlug})
		require.NoError(t, err)
		return res
	}

	assert.IsType(t, &siteapi.SiteWebhookEndpointResource{}, create(testhelper.Setup(t)))
	assert.IsType(t, new(siteapi.SiteWebhooksCreateUnprocessableEntity), create(testhelper.Setup(t, testhelper.WithoutLicense())))

	env := testhelper.Setup(t, testhelper.WithoutLicense())
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksUpdate(ctx,
		&siteapi.SiteUpdateWebhookEndpointInput{EventTypes: []string{"audit.entry"}},
		siteapi.SiteWebhooksUpdateParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteWebhooksUpdateUnprocessableEntity), res)
}
