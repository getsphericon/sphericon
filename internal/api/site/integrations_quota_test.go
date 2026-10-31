package site_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// reread reads an Integration's Provider limit back: discovery runs as a job after the
// save, so the save's own response does not carry its result (the test enqueuer runs
// the job inline, a real queue a moment later).
func reread(t *testing.T, c *siteapi.Client, id siteapi.EntityId) siteapi.SiteProviderLimitStatus {
	t.Helper()
	got, err := c.SiteIntegrationsGet(context.Background(), siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	res, ok := got.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", got)
	return res.ProviderLimit
}

// Saving an SES Integration schedules a read of the account's send quota: the provider
// values feed the effective ceiling (the lowest of manual and provider, manual on a tie)
// from the next read on.
func TestSiteIntegrationsSaveDiscoversTheSESQuota(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	env.SES.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "SES", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
		MaxPerDay: siteapi.NewOptNilMaxPerDay(1000),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	status := reread(t, c, created.(*siteapi.SiteIntegrationResource).ID)

	assert.Equal(t, int32(14), status.PerSecond.Limit.Or(0))
	assert.Equal(t, siteapi.SiteProviderLimitSourceProvider, status.PerSecond.Source.Or(""))
	assert.Equal(t, int32(1000), status.PerDay.Limit.Or(0), "a manual value below the provider's wins")
	assert.Equal(t, siteapi.SiteProviderLimitSourceManual, status.PerDay.Source.Or(""))
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

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "SES", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "the save succeeds although the lookup failed, got %T", created)
	status := reread(t, c, res.ID)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{
		siteapi.SiteProviderLimitWarningUnlimited, siteapi.SiteProviderLimitWarningProviderQuotaUnavailable,
	}, status.Warnings)
	assert.True(t, status.PerSecond.Limit.Null)

	// A manual value takes over as the only ceiling, and the warning stays.
	upd := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID}
	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
		MaxPerSecond: siteapi.NewOptNilMaxPerSecond(5),
	}, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0))
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningProviderQuotaUnavailable},
		status.Warnings, "one ceiling is enough to stop the unlimited warning; the quota warning stays")

	// The permission is granted and the Integration is saved again: the warning clears.
	resave := &siteapi.UpdateIntegrationInput{
		Config: siteapi.NewOptIntegrationConfigInput(sesInput("eu-west-1", "AKIA1234", "")),
	}
	env.SES.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Empty(t, status.Warnings)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0), "manual 5 is below the provider's 14")
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0))
	assert.Equal(t, siteapi.SiteProviderLimitSourceProvider, status.PerDay.Source.Or(""))

	// A later failed lookup keeps the last known provider values and only warns.
	env.SES.SetQuotaErr(errors.New("timeout"))
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, c, res.ID)
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0), "a blip must not lift a ceiling that was known")
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningProviderQuotaUnavailable}, status.Warnings)
}

// SMTP has no provider quota, so saving it asks nothing and never warns about one.
func TestSiteIntegrationsSMTPSaveDoesNotLookUpAQuota(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.SES.SetQuotaErr(errors.New("must not be asked"))

	created, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Name: "SMTP", Config: smtpInput("smtp.example.com", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningUnlimited},
		created.(*siteapi.SiteIntegrationResource).ProviderLimit.Warnings)
}
