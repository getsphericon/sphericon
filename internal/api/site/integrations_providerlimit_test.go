package site_test

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteIntegrationsProviderLimitRoundTrips(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// Unlimited by default: both limits read back as null.
	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 1})
	require.NoError(t, err)
	def := got.(*siteapi.SiteIntegrationResource)
	assert.True(t, def.MaxPerSecond.Null)
	assert.True(t, def.MaxPerDay.Null)

	upd := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: def.ID}
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
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Equal(t, int32(2), created.(*siteapi.SiteIntegrationResource).MaxPerSecond.Or(0))
}

// The contract bounds a limit to 1..10 000 per second and 1..100 000 000 per day:
// the server refuses anything outside it and changes nothing.
func TestSiteIntegrationsProviderLimitIsBounded(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	upd := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: 1}

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

	got, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 1})
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
	params := siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 1}

	// No limit: nothing effective, no source, an "unlimited" warning, and the two
	// fixture sends of the last 24 hours (a third is older, SES's is another row).
	got, err := c.SiteIntegrationsGet(ctx, params)
	require.NoError(t, err)
	status := got.(*siteapi.SiteIntegrationResource).ProviderLimit
	assert.True(t, status.PerSecond.Limit.Null)
	assert.True(t, status.PerSecond.Source.Null)
	assert.True(t, status.PerDay.Limit.Null)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningUnlimited}, status.Warnings)
	assert.Equal(t, int32(2), status.SentLast24h)

	// A manual limit is the effective one, sourced "manual", and the warning goes.
	upd, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(14),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: 1})
	require.NoError(t, err)
	status = upd.(*siteapi.SiteIntegrationResource).ProviderLimit
	assert.Equal(t, int32(14), status.PerSecond.Limit.Or(0))
	assert.Equal(t, siteapi.SiteProviderLimitSourceManual, status.PerSecond.Source.Or(""))
	assert.True(t, status.PerDay.Limit.Null, "an unset window stays unlimited")
	assert.Empty(t, status.Warnings, "one ceiling is enough: nothing to warn about")

	// The list carries the same status and each row's own usage.
	list, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	usage := map[siteapi.EntityId]int32{}
	for _, row := range *list.(*siteapi.SiteIntegrationsListOKApplicationJSON) {
		usage[row.ID] = row.ProviderLimit.SentLast24h
	}
	assert.Equal(t, int32(2), usage[1])
	assert.Equal(t, int32(1), usage[2])
}

// postboxInput is an SES Integration with its own endpoint: an SES-compatible service
// (Yandex Cloud Postbox) that has no quota call, so the user gives its Provider limit.
func postboxInput() siteapi.SiteIntegrationConfigInput {
	return siteapi.SiteIntegrationConfigInput{
		OneOf: siteapi.NewSiteSesConfigInputSiteIntegrationConfigInputSum(siteapi.SiteSesConfigInput{
			Kind: siteapi.SiteSesConfigInputKindSes, Region: "ru-central1", AccessKeyId: "AKIA1234",
			SecretAccessKey: "secret", From: "noreply@acme.test",
			Endpoint: siteapi.NewOptNilString("https://postbox.cloud.yandex.net"),
		}),
	}
}

// refusedFields is the fields a 422 names.
func refusedFields(t *testing.T, res any) []string {
	t.Helper()
	var problem siteapi.ProblemDetails
	switch r := res.(type) {
	case *siteapi.SiteIntegrationsCreateUnprocessableEntity:
		problem = siteapi.ProblemDetails(*r)
	case *siteapi.SiteIntegrationsUpdateUnprocessableEntity:
		problem = siteapi.ProblemDetails(*r)
	default:
		require.Failf(t, "not a 422", "%T", res)
	}
	return slices.Sorted(maps.Keys(problem.Errors.Value))
}

// An SES-compatible service needs both Provider limit values (ADR 0067): a create without
// either is refused and names the missing one.
func TestSiteIntegrationsSesCompatibleRequiresTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	for name, tc := range map[string]struct {
		in      siteapi.SiteCreateIntegrationInput
		missing []string
	}{
		"neither": {siteapi.SiteCreateIntegrationInput{}, []string{"maxPerDay", "maxPerSecond"}},
		"no daily": {siteapi.SiteCreateIntegrationInput{
			MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(1),
		}, []string{"maxPerDay"}},
		"null per second": {siteapi.SiteCreateIntegrationInput{
			MaxPerSecond: siteapi.OptNilSiteMaxPerSecond{Set: true, Null: true},
			MaxPerDay:    siteapi.NewOptNilSiteMaxPerDay(200),
		}, []string{"maxPerSecond"}},
	} {
		t.Run(name, func(t *testing.T) {
			in := tc.in
			in.Name, in.Params, in.Config = "Postbox", sesTopicParams, postboxInput()
			res, err := c.SiteIntegrationsCreate(ctx, &in, params)
			require.NoError(t, err)
			assert.Equal(t, tc.missing, refusedFields(t, res))
		})
	}

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "Postbox", Params: sesTopicParams, Config: postboxInput(),
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(1),
		MaxPerDay:    siteapi.NewOptNilSiteMaxPerDay(200),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, created, "both values: saved")
}

// An edit is judged by what it leaves: an Integration that gains an endpoint gets both values
// in the same write, and an SES-compatible one cannot lose either. Other edits pass.
func TestSiteIntegrationsSesCompatibleKeepsTheProviderLimit(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	// The fixture's AWS SES Integration has no endpoint and no limit.
	params := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.IntegrationAcmeSesID}

	moved, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(postboxInput()),
	}, params)
	require.NoError(t, err)
	assert.Equal(t, []string{"maxPerDay", "maxPerSecond"}, refusedFields(t, moved))

	movedWithLimits, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Config:       siteapi.NewOptNilSiteIntegrationConfigInput(postboxInput()),
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(1),
		MaxPerDay:    siteapi.NewOptNilSiteMaxPerDay(200),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, movedWithLimits)

	cleared, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerDay: siteapi.OptNilSiteMaxPerDay{Set: true, Null: true},
	}, params)
	require.NoError(t, err)
	assert.Equal(t, []string{"maxPerDay"}, refusedFields(t, cleared))

	renamed, err := c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		Name: siteapi.NewOptString("Postbox (prod)"),
	}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteIntegrationResource{}, renamed, "the stored values still apply")
}
