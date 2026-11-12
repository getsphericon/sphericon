package site_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// reread reads an Integration's Provider limit back once the quota reads the saves queued have
// run: discovery runs as a job after the save, so the save's own response does not carry its
// result.
func reread(t *testing.T, env *testhelper.TestEnv, c *siteapi.Client, id siteapi.EntityId) siteapi.SiteProviderLimitStatus {
	t.Helper()
	env.PerformEnqueuedJobs(t, integrations.RefreshQuotaArgs{})
	got, err := c.SiteIntegrationsGet(context.Background(), siteapi.SiteIntegrationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	res := got
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
	status := reread(t, env, c, created.ID)

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
	res := created
	status := reread(t, env, c, res.ID)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{
		siteapi.SiteProviderLimitWarningUnlimited, siteapi.SiteProviderLimitWarningProviderQuotaUnavailable,
	}, status.Warnings)
	assert.True(t, status.PerSecond.Limit.Null)

	// A manual value takes over as the only ceiling, and the warning stays.
	upd := siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID}
	_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(ctx, t, int64(upd.ID))),
		MaxPerSecond: siteapi.NewOptNilMaxPerSecond(5),
	}, upd)
	require.NoError(t, err)
	status = reread(t, env, c, res.ID)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0))
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningProviderQuotaUnavailable},
		status.Warnings, "one ceiling is enough to stop the unlimited warning; the quota warning stays")

	// The permission is granted and the Integration is saved again: the warning clears.
	resave := &siteapi.UpdateIntegrationInput{
		Revision: siteapi.Revision(env.IntegrationRevision(ctx, t, int64(res.ID))),
		Config:   siteapi.NewOptIntegrationConfigInput(sesInput("eu-west-1", "AKIA1234", "")),
	}
	env.SES.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, env, c, res.ID)
	assert.Empty(t, status.Warnings)
	assert.Equal(t, int32(5), status.PerSecond.Limit.Or(0), "manual 5 is below the provider's 14")
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0))
	assert.Equal(t, siteapi.SiteProviderLimitSourceProvider, status.PerDay.Source.Or(""))

	// A later failed lookup keeps the last known provider values and only warns.
	env.SES.SetQuotaErr(errors.New("timeout"))
	resave.Revision = siteapi.Revision(env.IntegrationRevision(ctx, t, int64(res.ID)))
	_, err = c.SiteIntegrationsUpdate(ctx, resave, upd)
	require.NoError(t, err)
	status = reread(t, env, c, res.ID)
	assert.Equal(t, int32(50000), status.PerDay.Limit.Or(0), "a blip must not lift a ceiling that was known")
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningProviderQuotaUnavailable}, status.Warnings)
}

// The read also brings the account's standing (ADR 0067): a stopped account, probation and
// the sandbox are reported as warnings on the Integration.
func TestSiteIntegrationsReportTheProviderAccountStanding(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	env.SES.SetQuota(messaging.Quota{PerSecond: new(1), Enforcement: providers.EnforcementShutdown, Sandbox: true})

	created, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.CreateIntegrationInput{
		Params: sesTopicParams,
		Name:   "SES", Config: sesInput("eu-west-1", "AKIA1234", "secret"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	id := created.ID
	status := reread(t, env, c, id)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{
		siteapi.SiteProviderLimitWarningProviderStopped, siteapi.SiteProviderLimitWarningProviderSandbox,
	}, status.Warnings)

	// New credentials may name another account: what the old ones read goes with them, so a
	// read that then fails holds nothing.
	env.SES.SetQuotaErr(errors.New("AccessDenied: ses:GetAccount"))
	_, err = c.SiteIntegrationsUpdate(context.Background(), &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(context.Background(), t, int64(id))),
		Config: siteapi.NewOptIntegrationConfigInput(sesInput("eu-west-1", "AKIA5678", "other")),
	}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: id})
	require.NoError(t, err)
	assert.Equal(t, []siteapi.SiteProviderLimitWarning{siteapi.SiteProviderLimitWarningProviderQuotaUnavailable},
		reread(t, env, c, id).Warnings)
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
		created.ProviderLimit.Warnings)
}
