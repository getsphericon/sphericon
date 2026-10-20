package sendlimit_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sendlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func envCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	c, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	return c
}

func reload(t *testing.T, s *ent.Scoped, id int64) *ent.Integration {
	t.Helper()
	row, err := s.Integration().Get(t.Context(), id)
	require.NoError(t, err)
	return row
}

func TestRefreshQuotaStoresTheProviderValuesAndFeedsTheCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})

	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, envCipher(t), ses.Catalog(), reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, new(14), row.ProviderMaxPerSecond)
	assert.Equal(t, new(50000), row.ProviderMaxPerDay)
	assert.False(t, row.ProviderQuotaUnavailable)
	assert.NotNil(t, row.ProviderQuotaCheckedAt)
	eff := sendlimit.EffectiveOf(row)
	assert.Equal(t, sendlimit.Value{Limit: new(14), Source: sendlimit.SourceProvider}, eff.PerSecond)

	// A manual value below the provider's wins as a value.
	require.NoError(t, s.Integration().UpdateOneID(row.ID).SetMaxPerSecond(5).Exec(t.Context()))
	eff = sendlimit.EffectiveOf(reload(t, s, row.ID))
	assert.Equal(t, sendlimit.Value{Limit: new(5), Source: sendlimit.SourceManual}, eff.PerSecond)
}

func TestRefreshQuotaFailureKeepsManualOrUnlimitedAndRecordsAWarning(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuotaErr(errors.New("AccessDenied: ses:GetSendQuota"))
	require.NoError(t, s.Integration().UpdateOneID(fixtures.IntegrationAcmeSesID).SetMaxPerDay(1000).Exec(t.Context()))

	err := sendlimit.RefreshQuota(t.Context(), s, envCipher(t), ses.Catalog(), reload(t, s, fixtures.IntegrationAcmeSesID))
	require.NoError(t, err, "a failed lookup is recorded, never returned: it must not fail a save or a job")

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.True(t, row.ProviderQuotaUnavailable)
	assert.NotNil(t, row.ProviderQuotaCheckedAt, "a failed lookup still counts as a check")
	eff := sendlimit.EffectiveOf(row)
	assert.True(t, eff.ProviderQuotaUnavailable)
	assert.Equal(t, sendlimit.Value{Limit: new(1000), Source: sendlimit.SourceManual}, eff.PerDay)
	assert.Equal(t, sendlimit.Value{}, eff.PerSecond, "no manual value and no provider value is unlimited")
}

func TestRefreshQuotaSuccessAfterFailureClearsTheWarning(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuotaErr(errors.New("AccessDenied"))
	cipher, catalog := envCipher(t), ses.Catalog()

	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, cipher, catalog, reload(t, s, fixtures.IntegrationAcmeSesID)))
	require.True(t, reload(t, s, fixtures.IntegrationAcmeSesID).ProviderQuotaUnavailable)

	ses.SetQuota(messaging.Quota{PerSecond: new(1)})
	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, cipher, catalog, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.False(t, row.ProviderQuotaUnavailable)
	assert.Equal(t, new(1), row.ProviderMaxPerSecond)
	assert.Nil(t, row.ProviderMaxPerDay)
}

func TestRefreshQuotaFailureKeepsTheLastKnownProviderValues(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	cipher, catalog := envCipher(t), ses.Catalog()
	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, cipher, catalog, reload(t, s, fixtures.IntegrationAcmeSesID)))

	ses.SetQuotaErr(errors.New("timeout"))
	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, cipher, catalog, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.True(t, row.ProviderQuotaUnavailable)
	assert.Equal(t, new(14), row.ProviderMaxPerSecond, "a transient blip must not lift a ceiling that was known")
}

func TestRefreshQuotaIgnoresProvidersThatCannotReportOne(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}

	require.NoError(t, sendlimit.RefreshQuota(t.Context(), s, envCipher(t), ses.Catalog(), reload(t, s, fixtures.IntegrationAcmeDefaultID)))

	row := reload(t, s, fixtures.IntegrationAcmeDefaultID)
	assert.False(t, row.ProviderQuotaUnavailable, "SMTP has no quota to be unavailable")
	assert.Nil(t, row.ProviderQuotaCheckedAt)
	assert.Zero(t, ses.QuotaCalls())
}
