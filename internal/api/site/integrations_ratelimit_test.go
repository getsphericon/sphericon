package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteIntegrationsSendRateLimitRoundTrips(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Unlimited by default: both limits read back as null.
	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{Slug: fixtures.AcmeSlug, ID: 1})
	require.NoError(t, err)
	def := got.(*siteapi.SiteIntegrationResource)
	assert.True(t, def.MaxPerSecond.Null)
	assert.True(t, def.MaxPerDay.Null)

	upd := siteapi.SiteIntegrationsUpdateParams{Slug: fixtures.AcmeSlug, ID: def.ID}
	updated, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(14),
		MaxPerDay:    siteapi.NewOptNilSiteMaxPerDay(50000),
	}, upd)
	require.NoError(t, err)
	res := updated.(*siteapi.SiteIntegrationResource)
	assert.Equal(t, int32(14), res.MaxPerSecond.Or(0))
	assert.Equal(t, int32(50000), res.MaxPerDay.Or(0))

	// Omitting a limit keeps it; null clears it.
	partial, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerDay: siteapi.OptNilSiteMaxPerDay{Set: true, Null: true},
	}, upd)
	require.NoError(t, err)
	partialRes := partial.(*siteapi.SiteIntegrationResource)
	assert.Equal(t, int32(14), partialRes.MaxPerSecond.Or(0), "omitted: unchanged")
	assert.True(t, partialRes.MaxPerDay.Null, "null: cleared")

	// Create accepts the limits too.
	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "Limited SMTP", Config: smtpInput("smtp.example.com", "pw"),
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(2),
	}, siteapi.SiteIntegrationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.Equal(t, int32(2), created.(*siteapi.SiteIntegrationResource).MaxPerSecond.Or(0))
}

// The contract bounds a limit to 1..10 000 per second and 1..100 000 000 per day:
// the server refuses anything outside it and changes nothing.
func TestSiteIntegrationsSendRateLimitIsBounded(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	upd := siteapi.SiteIntegrationsUpdateParams{Slug: fixtures.AcmeSlug, ID: 1}

	for name, in := range map[string]*siteapi.SiteUpdateIntegrationInput{
		"zero per second":   {MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(0)},
		"negative per day":  {MaxPerDay: siteapi.NewOptNilSiteMaxPerDay(-5)},
		"too fast":          {MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(10001)},
		"too many in a day": {MaxPerDay: siteapi.NewOptNilSiteMaxPerDay(100000001)},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := c.SiteIntegrationsUpdate(ctx, in, upd)
			require.NoError(t, err)
			assert.IsType(t, &siteapi.SiteIntegrationsUpdateBadRequest{}, res)
		})
	}

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{Slug: fixtures.AcmeSlug, ID: 1})
	require.NoError(t, err)
	assert.True(t, got.(*siteapi.SiteIntegrationResource).MaxPerSecond.Null, "a rejected update changed nothing")

	edge, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(10000),
		MaxPerDay:    siteapi.NewOptNilSiteMaxPerDay(100000000),
	}, upd)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, edge, "the bounds themselves are allowed")
}

func TestSiteIntegrationsReportEffectiveLimitAndUsage(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsGetParams{Slug: fixtures.AcmeSlug, ID: 1}

	// No limit: nothing effective, no source, an "unlimited" warning, and the two
	// fixture sends of the last 24 hours (a third is older, SES's is another row).
	got, err := c.SiteIntegrationsGet(ctx, params)
	require.NoError(t, err)
	status := got.(*siteapi.SiteIntegrationResource).SendLimit
	assert.True(t, status.PerSecond.Limit.Null)
	assert.True(t, status.PerSecond.Source.Null)
	assert.True(t, status.PerDay.Limit.Null)
	assert.Equal(t, []siteapi.SiteSendLimitWarning{siteapi.SiteSendLimitWarningUnlimited}, status.Warnings)
	assert.Equal(t, int32(2), status.SentLast24h)

	// A manual limit is the effective one, sourced "manual", and the warning goes.
	upd, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(14),
	}, siteapi.SiteIntegrationsUpdateParams{Slug: fixtures.AcmeSlug, ID: 1})
	require.NoError(t, err)
	status = upd.(*siteapi.SiteIntegrationResource).SendLimit
	assert.Equal(t, int32(14), status.PerSecond.Limit.Or(0))
	assert.Equal(t, siteapi.SiteSendLimitSourceManual, status.PerSecond.Source.Or(""))
	assert.True(t, status.PerDay.Limit.Null, "an unset window stays unlimited")
	assert.Empty(t, status.Warnings, "one ceiling is enough: nothing to warn about")

	// The list carries the same status and each row's own usage.
	list, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	usage := map[siteapi.EntityId]int32{}
	for _, row := range *list.(*siteapi.SiteIntegrationsListOKApplicationJSON) {
		usage[row.ID] = row.SendLimit.SentLast24h
	}
	assert.Equal(t, map[siteapi.EntityId]int32{1: 2, 2: 1}, usage)
}
