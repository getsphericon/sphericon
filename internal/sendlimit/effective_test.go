package sendlimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/sendlimit"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestEffectiveOfReportsManualValuesAndTheirSource(t *testing.T) {
	perSecond, perDay := 14, 50000

	none := sendlimit.EffectiveOf(&ent.Integration{})
	assert.True(t, none.Unlimited())
	assert.Nil(t, none.PerSecond.Limit)
	assert.Empty(t, none.PerSecond.Source, "no ceiling has no source")

	both := sendlimit.EffectiveOf(&ent.Integration{MaxPerSecond: &perSecond, MaxPerDay: &perDay})
	assert.False(t, both.Unlimited())
	assert.Equal(t, sendlimit.Value{Limit: &perSecond, Source: sendlimit.SourceManual}, both.PerSecond)
	assert.Equal(t, sendlimit.Value{Limit: &perDay, Source: sendlimit.SourceManual}, both.PerDay)
	assert.Equal(t, sendlimit.Limits{PerSecond: &perSecond, PerDay: &perDay}, both.Limits())

	onlyDay := sendlimit.EffectiveOf(&ent.Integration{MaxPerDay: &perDay})
	assert.False(t, onlyDay.Unlimited(), "one ceiling is enough to be limited")
}

func TestEffectiveOfTakesTheLowestOfManualAndProviderValues(t *testing.T) {
	cases := []struct {
		name             string
		manual, provider *int
		want             sendlimit.Value
	}{
		{"neither", nil, nil, sendlimit.Value{}},
		{"provider only", nil, new(14), sendlimit.Value{Limit: new(14), Source: sendlimit.SourceProvider}},
		{"manual only", new(5), nil, sendlimit.Value{Limit: new(5), Source: sendlimit.SourceManual}},
		{"manual is lower", new(5), new(14), sendlimit.Value{Limit: new(5), Source: sendlimit.SourceManual}},
		{"provider is lower", new(50), new(14), sendlimit.Value{Limit: new(14), Source: sendlimit.SourceProvider}},
		{"manual wins a tie", new(14), new(14), sendlimit.Value{Limit: new(14), Source: sendlimit.SourceManual}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sendlimit.EffectiveOf(&ent.Integration{
				MaxPerSecond: c.manual, ProviderMaxPerSecond: c.provider,
				MaxPerDay: c.manual, ProviderMaxPerDay: c.provider,
			})
			assert.Equal(t, c.want, got.PerSecond)
			assert.Equal(t, c.want, got.PerDay)
		})
	}
}

func TestEffectiveOfReportsAnUnavailableProviderQuota(t *testing.T) {
	assert.False(t, sendlimit.EffectiveOf(&ent.Integration{}).ProviderQuotaUnavailable)
	assert.True(t, sendlimit.EffectiveOf(&ent.Integration{ProviderQuotaFailedAt: new(time.Now())}).ProviderQuotaUnavailable)
}

func TestUsageCountsSentMessagesPerIntegrationOverTheLastDay(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := sendlimit.Usage(ctx, env.DB.Scoped(fixtures.AcmeID), time.Now())
	require.NoError(t, err)
	assert.Equal(t, map[int64]int{
		fixtures.IntegrationAcmeDefaultID: 2, // the third send is 30 hours old
		fixtures.IntegrationAcmeSesID:     1,
	}, got, "the pending, unstamped and Globex rows do not count")
}

func TestWarningsListsWhatIsWorthFlaggedInAStableOrder(t *testing.T) {
	limited := sendlimit.Value{Limit: new(5), Source: sendlimit.SourceManual}
	tests := []struct {
		name string
		eff  sendlimit.Effective
		want []sendlimit.Warning
	}{
		{"nothing to flag", sendlimit.Effective{PerSecond: limited}, []sendlimit.Warning{}},
		{"no ceiling at all", sendlimit.Effective{}, []sendlimit.Warning{sendlimit.WarningUnlimited}},
		{"provider quota unreadable", sendlimit.Effective{PerDay: limited, ProviderQuotaUnavailable: true}, []sendlimit.Warning{sendlimit.WarningProviderQuotaUnavailable}},
		{"both", sendlimit.Effective{ProviderQuotaUnavailable: true}, []sendlimit.Warning{sendlimit.WarningUnlimited, sendlimit.WarningProviderQuotaUnavailable}},
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
	usage, err := sendlimit.Usage(ctx, s, time.Now())
	require.NoError(t, err)

	got, err := sendlimit.Statuses(ctx, s, rows, time.Now())
	require.NoError(t, err)

	require.Len(t, got, len(rows))
	for _, row := range rows {
		eff := sendlimit.EffectiveOf(row)
		assert.Equal(t, sendlimit.Status{PerSecond: eff.PerSecond, PerDay: eff.PerDay, Sent24h: usage[row.ID], Warnings: eff.Warnings()}, got[row.ID])
		for _, v := range []sendlimit.Value{got[row.ID].PerSecond, got[row.ID].PerDay} {
			assert.Equal(t, v.Limit == nil, v.Source == "", "a limit has a source exactly when it is set")
		}
	}
}
