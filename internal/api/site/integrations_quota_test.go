package site_test

import (
	"context"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// reread reads an Integration's Send rate limit back: discovery runs as a job after the
// save, so the save's own response does not carry its result (the test enqueuer runs
// the job inline, a real queue a moment later).
func reread(t *testing.T, c *siteapi.Client, id siteapi.EntityId) siteapi.SiteSendLimitStatus {
	t.Helper()
	got, err := c.SiteIntegrationsGet(context.Background(), siteapi.SiteIntegrationsGetParams{Slug: fixtures.AcmeSlug, ID: id})
	require.NoError(t, err)
	res, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", got)
	return res.SendLimit
}

// Saving an SES Integration schedules a read of the account's send quota: the provider
// values feed the effective ceiling (the lowest of manual and provider, manual on a tie)
// from the next read on.
func TestSiteIntegrationsSaveDiscoversTheSESQuota(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	env.SES.SetQuota(messaging.Quota{PerSecond: lo.ToPtr(14), PerDay: lo.ToPtr(50000)})

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "SES", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
		MaxPerDay: siteapi.NewOptNilSiteMaxPerDay(1000),
	}, siteapi.SiteIntegrationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	status := reread(t, c, created.(*siteapi.SiteIntegrationResource).ID)

	assert.Equal(t, int32(14), status.PerSecond.Limit.Or(0))
	assert.Equal(t, siteapi.SiteSendLimitSourceProvider, status.PerSecond.Source.Or(""))
	assert.Equal(t, int32(1000), status.PerDay.Limit.Or(0), "a manual value below the provider's wins")
	assert.Equal(t, siteapi.SiteSendLimitSourceManual, status.PerDay.Source.Or(""))
	assert.Empty(t, status.Warnings)
}

// A failed lookup never blocks the save or sending: the manual value (or none) applies
// and a warning is reported until a later save, or the hourly job, succeeds. A failure
// after a success keeps the last known provider values.
func TestSiteIntegrationsFailedQuotaLookupWarnsAndLaterSuccessClearsIt(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	env.SES.SetQuotaErr(errors.New("AccessDenied: ses:GetSendQuota"))

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "SES", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, siteapi.SiteIntegrationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "the save succeeds although the lookup failed, got %T", created)
	status := reread(t, c, res.ID)
	assert.Equal(t, []siteapi.SiteSendLimitWarning{
		siteapi.SiteSendLimitWarningUnlimited, siteapi.SiteSendLimitWarningProviderQuotaUnavailable,
	}, status.Warnings)
	assert.True(t, status.PerSecond.Limit.Null)

	// A manual value takes over as the only ceiling, and the warning stays.
	upd := siteapi.SiteIntegrationsUpdateParams{Slug: fixtures.AcmeSlug, ID: res.ID}
	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilSiteMaxPerSecond(5),
	}, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0))
	assert.Equal(t, []siteapi.SiteSendLimitWarning{siteapi.SiteSendLimitWarningProviderQuotaUnavailable},
		status.Warnings, "one ceiling is enough to stop the unlimited warning; the quota warning stays")

	// The permission is granted and the Integration is saved again: the warning clears.
	resave := &siteapi.SiteUpdateIntegrationInput{
		Config: siteapi.NewOptNilSiteIntegrationConfigInput(sesInput("eu-west-1", "AKIA1234", "")),
	}
	env.SES.SetQuota(messaging.Quota{PerSecond: lo.ToPtr(14), PerDay: lo.ToPtr(50000)})
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Empty(t, status.Warnings)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0), "manual 5 is below the provider's 14")
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0))
	assert.Equal(t, siteapi.SiteSendLimitSourceProvider, status.PerDay.Source.Or(""))

	// A later failed lookup keeps the last known provider values and only warns.
	env.SES.SetQuotaErr(errors.New("timeout"))
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0), "a blip must not lift a ceiling that was known")
	assert.Equal(t, []siteapi.SiteSendLimitWarning{siteapi.SiteSendLimitWarningProviderQuotaUnavailable}, status.Warnings)
}

// SMTP has no provider quota, so saving it asks nothing and never warns about one.
func TestSiteIntegrationsSMTPSaveDoesNotLookUpAQuota(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.SES.SetQuotaErr(errors.New("must not be asked"))

	created, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.SiteCreateIntegrationInput{
		Name: "SMTP", Config: smtpInput("smtp.example.com", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.Equal(t, []siteapi.SiteSendLimitWarning{siteapi.SiteSendLimitWarningUnlimited},
		created.(*siteapi.SiteIntegrationResource).SendLimit.Warnings)
	assert.Zero(t, env.SES.QuotaCalls())
}
