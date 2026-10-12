package jobs_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sendlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// envCipher is the cipher over the test environment's ENCRYPTION_KEY, the key the
// fixtures were sealed with, so a fixture Integration's config decrypts.
func envCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	return c
}

func intPtr(n int) *int { return &n }

func TestRefreshSendQuotasWorkerFansOutOneJobPerSESIntegration(t *testing.T) {
	e := newRiverEnv(t)
	ses, err := e.DB.Integration.Query().Where(integration.ProviderEQ(integration.ProviderSes)).Count(t.Context())
	require.NoError(t, err)
	require.Positive(t, ses)

	require.NoError(t, jobs.NewRefreshSendQuotasWorker(e.DB).Work(e.workCtx(), job(jobs.RefreshSendQuotasArgs{})))

	kinds := e.queued(t)
	assert.Len(t, kinds, ses, "SMTP Integrations report no quota and are not queued")
	for _, k := range kinds {
		assert.Equal(t, "integration_quota_refresh", k)
	}
}

func TestRefreshIntegrationQuotaFollowsAccountGrowthAndRecovers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	fake := &testhelper.FakeSES{}
	fake.SetQuotaErr(errors.New("AccessDenied: ses:GetSendQuota"))
	worker := jobs.NewRefreshIntegrationQuotaWorker(env.DB, envCipher(t), fake.Catalog())
	run := func() {
		require.NoError(t, worker.Work(ctx, job(jobs.RefreshIntegrationQuotaArgs{IntegrationID: fixtures.IntegrationAcmeSesID})))
	}
	effective := func() sendlimit.Effective {
		row, err := s.Integration().Get(ctx, fixtures.IntegrationAcmeSesID)
		require.NoError(t, err)
		return sendlimit.EffectiveOf(row)
	}

	// The lookup fails: unlimited with a warning, and the job still succeeds.
	run()
	assert.True(t, effective().ProviderQuotaUnavailable)
	assert.True(t, effective().Unlimited())

	// A sandbox account, then the account matures: each hourly run follows it.
	fake.SetQuota(messaging.Quota{PerSecond: intPtr(1), PerDay: intPtr(200)})
	run()
	eff := effective()
	assert.False(t, eff.ProviderQuotaUnavailable, "a later success clears the warning")
	assert.Equal(t, sendlimit.Value{Limit: intPtr(1), Source: sendlimit.SourceProvider}, eff.PerSecond)

	fake.SetQuota(messaging.Quota{PerSecond: intPtr(14), PerDay: intPtr(50000)})
	run()
	assert.Equal(t, sendlimit.Value{Limit: intPtr(14), Source: sendlimit.SourceProvider}, effective().PerSecond)
	assert.Equal(t, sendlimit.Value{Limit: intPtr(50000), Source: sendlimit.SourceProvider}, effective().PerDay)
}

func TestRefreshIntegrationQuotaOfADeletedIntegrationIsNothingToDo(t *testing.T) {
	env := testhelper.Setup(t)
	worker := jobs.NewRefreshIntegrationQuotaWorker(env.DB, envCipher(t), (&testhelper.FakeSES{}).Catalog())
	require.NoError(t, worker.Work(t.Context(), job(jobs.RefreshIntegrationQuotaArgs{IntegrationID: 424242})))
}

func TestIntegrationsDueForQuotaRefreshAreTheSESOnes(t *testing.T) {
	env := testhelper.Setup(t)
	ids, err := jobs.IntegrationsDueForQuotaRefresh(t.Context(), env.DB, 100)
	require.NoError(t, err)
	assert.Contains(t, ids, int64(fixtures.IntegrationAcmeSesID))
	assert.NotContains(t, ids, int64(fixtures.IntegrationAcmeDefaultID))
}
