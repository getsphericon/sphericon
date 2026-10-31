package providerlimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestEffectiveOfReportsManualValuesAndTheirSource(t *testing.T) {
	perSecond, perDay := 14, 50000

	none := providerlimit.EffectiveOf(&ent.Integration{})
	assert.True(t, none.Unlimited())
	assert.Nil(t, none.PerSecond.Limit)
	assert.Empty(t, none.PerSecond.Source, "no ceiling has no source")

	both := providerlimit.EffectiveOf(&ent.Integration{MaxPerSecond: &perSecond, MaxPerDay: &perDay})
	assert.False(t, both.Unlimited())
	assert.Equal(t, providerlimit.Value{Limit: &perSecond, Source: providerlimit.SourceManual}, both.PerSecond)
	assert.Equal(t, providerlimit.Value{Limit: &perDay, Source: providerlimit.SourceManual}, both.PerDay)
	assert.Equal(t, providerlimit.Limits{PerSecond: &perSecond, PerDay: &perDay}, both.Limits())

	onlyDay := providerlimit.EffectiveOf(&ent.Integration{MaxPerDay: &perDay})
	assert.False(t, onlyDay.Unlimited(), "one ceiling is enough to be limited")
}

func TestEffectiveOfTakesTheLowestOfManualAndProviderValues(t *testing.T) {
	cases := []struct {
		name             string
		manual, provider *int
		want             providerlimit.Value
	}{
		{"neither", nil, nil, providerlimit.Value{}},
		{"provider only", nil, new(14), providerlimit.Value{Limit: new(14), Source: providerlimit.SourceProvider}},
		{"manual only", new(5), nil, providerlimit.Value{Limit: new(5), Source: providerlimit.SourceManual}},
		{"manual is lower", new(5), new(14), providerlimit.Value{Limit: new(5), Source: providerlimit.SourceManual}},
		{"provider is lower", new(50), new(14), providerlimit.Value{Limit: new(14), Source: providerlimit.SourceProvider}},
		{"manual wins a tie", new(14), new(14), providerlimit.Value{Limit: new(14), Source: providerlimit.SourceManual}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := providerlimit.EffectiveOf(&ent.Integration{
				MaxPerSecond: c.manual, ProviderMaxPerSecond: c.provider,
				MaxPerDay: c.manual, ProviderMaxPerDay: c.provider,
			})
			assert.Equal(t, c.want, got.PerSecond)
			assert.Equal(t, c.want, got.PerDay)
		})
	}
}

func TestEffectiveOfReportsAnUnavailableProviderQuota(t *testing.T) {
	assert.False(t, providerlimit.EffectiveOf(&ent.Integration{}).ProviderQuotaUnavailable)
	assert.True(t, providerlimit.EffectiveOf(&ent.Integration{ProviderQuotaFailedAt: new(time.Now())}).ProviderQuotaUnavailable)
}

func TestUsageCountsSentMessagesPerIntegrationOverTheLastDay(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := providerlimit.Usage(ctx, env.DB.Scoped(fixtures.AcmeID), time.Now())
	require.NoError(t, err)
	assert.Equal(t, map[int64]int{
		fixtures.IntegrationAcmeDefaultID: 2, // the third send is 30 hours old
		fixtures.IntegrationAcmeSesID:     1,
	}, got, "the pending, unstamped and Globex rows do not count")
}

func TestWarningsListsWhatIsWorthFlaggedInAStableOrder(t *testing.T) {
	limited := providerlimit.Value{Limit: new(5), Source: providerlimit.SourceManual}
	tests := []struct {
		name string
		eff  providerlimit.Effective
		want []providerlimit.Warning
	}{
		{"nothing to flag", providerlimit.Effective{PerSecond: limited}, []providerlimit.Warning{}},
		{"no ceiling at all", providerlimit.Effective{}, []providerlimit.Warning{providerlimit.WarningUnlimited}},
		{"provider quota unreadable", providerlimit.Effective{PerDay: limited, ProviderQuotaUnavailable: true}, []providerlimit.Warning{providerlimit.WarningProviderQuotaUnavailable}},
		{"both", providerlimit.Effective{ProviderQuotaUnavailable: true}, []providerlimit.Warning{providerlimit.WarningUnlimited, providerlimit.WarningProviderQuotaUnavailable}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.eff.Warnings()

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStatusesPairUsageWithTheEffectiveLimits(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()

	rows, err := s.Integration().Query().All(ctx)
	require.NoError(t, err)
	usage, err := providerlimit.Usage(ctx, s, time.Now())
	require.NoError(t, err)

	got, err := providerlimit.Statuses(ctx, s, rows, time.Now())
	require.NoError(t, err)

	require.Len(t, got, len(rows))
	for _, row := range rows {
		eff := providerlimit.EffectiveOf(row)
		assert.Equal(t, providerlimit.Status{PerSecond: eff.PerSecond, PerDay: eff.PerDay, Sent24h: usage[row.ID], Warnings: eff.Warnings()}, got[row.ID])
		for _, v := range []providerlimit.Value{got[row.ID].PerSecond, got[row.ID].PerDay} {
			assert.Equal(t, v.Limit == nil, v.Source == "", "a limit has a source exactly when it is set")
		}
	}
}
