package integrations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// quotaModule is the module over the provider catalog a test scripts.
func quotaModule(cipher *secrets.Cipher, catalog *messaging.Catalog) *integrations.Module {
	return integrations.New(nil, cipher, catalog, nil, "", nil, providerlimit.New(time.Now), time.Now)
}

func reload(t *testing.T, s *ent.Scoped, id int64) *ent.Integration {
	t.Helper()
	row, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).WithProviderState().Only(t.Context())
	require.NoError(t, err)
	return row
}

func TestRefreshQuotaStoresTheProviderValuesAndFeedsTheCeiling(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})

	require.NoError(t, quotaModule(env.Cipher, ses.Catalog()).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, new(14), row.Edges.ProviderState.ProviderMaxPerSecond)
	assert.Equal(t, new(50000), row.Edges.ProviderState.ProviderMaxPerDay)
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaFailedAt)
	assert.NotNil(t, row.Edges.ProviderState.ProviderQuotaCheckedAt)
	eff := providerlimit.EffectiveOf(row)
	assert.Equal(t, providerlimit.Value{Limit: new(14), Source: providerlimit.SourceProvider}, eff.PerSecond)

	// A manual value below the provider's wins as a value.
	require.NoError(t, s.Integration().UpdateOneID(row.ID).SetMaxPerSecond(5).Exec(t.Context()))
	eff = providerlimit.EffectiveOf(reload(t, s, row.ID))
	assert.Equal(t, providerlimit.Value{Limit: new(5), Source: providerlimit.SourceManual}, eff.PerSecond)
}

func TestRefreshQuotaFailureKeepsManualOrUnlimitedAndRecordsAWarning(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuotaErr(errors.New("AccessDenied: ses:GetSendQuota"))
	require.NoError(t, s.Integration().UpdateOneID(fixtures.IntegrationAcmeSesID).SetMaxPerDay(1000).Exec(t.Context()))
	logs := testhelper.CaptureLogs(t)

	err := quotaModule(env.Cipher, ses.Catalog()).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID))
	require.NoError(t, err, "a failed lookup is recorded, never returned: it must not fail a save or a job")
	logged := logs.String()
	assert.Contains(t, logged, "provider send quota unavailable", "the operator sees why the warning is up")
	assert.Contains(t, logged, "AccessDenied: ses:GetSendQuota")

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.NotNil(t, row.Edges.ProviderState.ProviderQuotaFailedAt)
	assert.NotNil(t, row.Edges.ProviderState.ProviderQuotaCheckedAt, "a failed lookup still counts as a check")
	eff := providerlimit.EffectiveOf(row)
	assert.True(t, eff.ProviderQuotaUnavailable)
	assert.Equal(t, providerlimit.Value{Limit: new(1000), Source: providerlimit.SourceManual}, eff.PerDay)
	assert.Equal(t, providerlimit.Value{}, eff.PerSecond, "no manual value and no provider value is unlimited")
}

func TestRefreshQuotaKeepsTheMomentOfTheFirstFailure(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuotaErr(errors.New("AccessDenied"))
	cipher, catalog := env.Cipher, ses.Catalog()
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))
	first := reload(t, s, fixtures.IntegrationAcmeSesID)

	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, first))

	second := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Equal(t, first.Edges.ProviderState.ProviderQuotaFailedAt, second.Edges.ProviderState.ProviderQuotaFailedAt, "a further failure keeps the first moment")
	assert.True(t, second.Edges.ProviderState.ProviderQuotaCheckedAt.After(*first.Edges.ProviderState.ProviderQuotaCheckedAt), "the last check still moves")
}

func TestRefreshQuotaSuccessAfterFailureClearsTheWarning(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuotaErr(errors.New("AccessDenied"))
	cipher, catalog := env.Cipher, ses.Catalog()

	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))
	require.NotNil(t, reload(t, s, fixtures.IntegrationAcmeSesID).Edges.ProviderState.ProviderQuotaFailedAt)

	ses.SetQuota(messaging.Quota{PerSecond: new(1)})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaFailedAt)
	assert.Equal(t, new(1), row.Edges.ProviderState.ProviderMaxPerSecond)
	assert.Nil(t, row.Edges.ProviderState.ProviderMaxPerDay)
}

func TestRefreshQuotaFailureKeepsTheLastKnownProviderValues(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	cipher, catalog := env.Cipher, ses.Catalog()
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	ses.SetQuotaErr(errors.New("timeout"))
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.NotNil(t, row.Edges.ProviderState.ProviderQuotaFailedAt)
	assert.Equal(t, new(14), row.Edges.ProviderState.ProviderMaxPerSecond, "a transient blip must not lift a ceiling that was known")
}

func TestRefreshQuotaIgnoresProvidersThatCannotReportOne(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}

	require.NoError(t, quotaModule(env.Cipher, ses.Catalog()).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeDefaultID)))

	row := reload(t, s, fixtures.IntegrationAcmeDefaultID)
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaFailedAt, "SMTP has no quota to fail")
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaCheckedAt)
}

func TestRefreshQuotaClearsAProviderValueTheProviderStoppedReporting(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	cipher, catalog := env.Cipher, ses.Catalog()
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	ses.SetQuota(messaging.Quota{PerSecond: new(14)})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))
	withoutDay := reload(t, s, fixtures.IntegrationAcmeSesID)

	ses.SetQuota(messaging.Quota{PerDay: new(50000)})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))
	withoutSecond := reload(t, s, fixtures.IntegrationAcmeSesID)

	assert.Nil(t, withoutDay.Edges.ProviderState.ProviderMaxPerDay, "a daily ceiling the provider no longer reports is lifted, not kept")
	assert.Equal(t, new(14), withoutDay.Edges.ProviderState.ProviderMaxPerSecond)
	assert.Nil(t, withoutSecond.Edges.ProviderState.ProviderMaxPerSecond, "so is a per-second one")
	assert.Equal(t, new(50000), withoutSecond.Edges.ProviderState.ProviderMaxPerDay)
}

func TestRefreshQuotaReturnsAnErrorWhenTheStoredConfigCannotBeRead(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	ses.SetQuota(messaging.Quota{PerSecond: new(14), PerDay: new(50000)})
	integ := reload(t, s, fixtures.IntegrationAcmeSesID)
	integ.ConfigEncrypted = "not a ciphertext"

	err := quotaModule(env.Cipher, ses.Catalog()).RefreshQuota(t.Context(), s, integ)

	require.ErrorContains(t, err, "decrypt integration")
	require.ErrorIs(t, err, secrets.ErrCiphertext)
	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Nil(t, row.Edges.ProviderState.ProviderMaxPerSecond, "no provider value is stored from a config that could not be read")
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaCheckedAt)
}

func TestRefreshQuotaStoresTheAccountStanding(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	cipher, catalog := env.Cipher, ses.Catalog()
	ses.SetQuota(messaging.Quota{Enforcement: providers.EnforcementProbation, Sandbox: true})

	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	require.NotNil(t, row.Edges.ProviderState.ProviderEnforcement)
	assert.Equal(t, providers.EnforcementProbation, *row.Edges.ProviderState.ProviderEnforcement)
	assert.True(t, row.Edges.ProviderState.ProviderSandbox)
	assert.Equal(t, []providerlimit.Warning{
		providerlimit.WarningUnlimited, providerlimit.WarningProviderProbation, providerlimit.WarningProviderSandbox,
	}, providerlimit.EffectiveOf(row).Warnings())
}

func TestRefreshQuotaClearsTheStandingWhenTheAccountRecovers(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	cipher, catalog := env.Cipher, ses.Catalog()
	ses.SetQuota(messaging.Quota{Enforcement: providers.EnforcementShutdown, Sandbox: true})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	ses.SetQuota(messaging.Quota{})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Nil(t, row.Edges.ProviderState.ProviderEnforcement)
	assert.False(t, row.Edges.ProviderState.ProviderSandbox)
}

func TestRefreshQuotaFailureKeepsTheLastKnownStanding(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ses := &testhelper.FakeSES{}
	cipher, catalog := env.Cipher, ses.Catalog()
	ses.SetQuota(messaging.Quota{Enforcement: providers.EnforcementShutdown, Sandbox: true})
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	ses.SetQuotaErr(errors.New("timeout"))
	require.NoError(t, quotaModule(cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID)))

	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	require.NotNil(t, row.Edges.ProviderState.ProviderEnforcement, "a failed read says nothing about the account, so the last answer stands")
	assert.Equal(t, providers.EnforcementShutdown, *row.Edges.ProviderState.ProviderEnforcement)
	assert.True(t, row.Edges.ProviderState.ProviderSandbox)
}

// failStateWrites makes every write to an Integration's provider state that which picks fail with
// err.
func failStateWrites(env *testhelper.TestEnv, which func(*ent.IntegrationProviderStateMutation) bool, err error) {
	env.DB.IntegrationProviderState.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if state, ok := m.(*ent.IntegrationProviderStateMutation); ok && which(state) {
				return nil, err
			}
			return next.Mutate(ctx, m)
		})
	})
}

// A write RefreshQuota cannot make is returned, whichever it is: the stored answer of a lookup or
// of its failure, or the mark or the clearing of a failure (each guarded by its state).
func TestRefreshQuotaReturnsAWriteItCannotMake(t *testing.T) {
	injected := errors.New("injected failure")
	checked := func(m *ent.IntegrationProviderStateMutation) bool { _, ok := m.ProviderQuotaCheckedAt(); return ok }
	for name, tc := range map[string]struct {
		which     func(*ent.IntegrationProviderStateMutation) bool
		lookupErr error
	}{
		"the provider values":          {checked, nil},
		"the check of a failed lookup": {checked, errors.New("timeout")},
		"the clearing of a failure": {
			func(m *ent.IntegrationProviderStateMutation) bool { return m.ProviderQuotaFailedAtCleared() }, nil,
		},
		"the mark of a failure": {
			func(m *ent.IntegrationProviderStateMutation) bool { _, ok := m.ProviderQuotaFailedAt(); return ok }, errors.New("timeout"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			s := env.DB.Scoped(fixtures.AcmeID)
			ses := &testhelper.FakeSES{}
			ses.SetQuota(messaging.Quota{PerSecond: new(14)})
			ses.SetQuotaErr(tc.lookupErr)
			integ := reload(t, s, fixtures.IntegrationAcmeSesID)
			failStateWrites(env, tc.which, injected)

			err := quotaModule(env.Cipher, ses.Catalog()).RefreshQuota(t.Context(), s, integ)

			require.ErrorIs(t, err, injected)
		})
	}
}

// A stored config the provider cannot build a sender from is an error, not a provider without a
// quota: nothing is written.
func TestRefreshQuotaReturnsAConfigTheProviderCannotBuildFrom(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	unbuildable := errors.New("unreadable config")
	catalog := messaging.NewCatalog(messaging.ProviderDescriptor{
		Provider: providers.SES,
		Build:    func([]byte, messaging.Deps) (any, error) { return nil, unbuildable },
	})

	err := quotaModule(env.Cipher, catalog).RefreshQuota(t.Context(), s, reload(t, s, fixtures.IntegrationAcmeSesID))

	require.ErrorIs(t, err, unbuildable)
	row := reload(t, s, fixtures.IntegrationAcmeSesID)
	assert.Nil(t, row.Edges.ProviderState.ProviderQuotaCheckedAt)
}
