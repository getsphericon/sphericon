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
		{"provider only", nil, ptr(14), sendlimit.Value{Limit: ptr(14), Source: sendlimit.SourceProvider}},
		{"manual only", ptr(5), nil, sendlimit.Value{Limit: ptr(5), Source: sendlimit.SourceManual}},
		{"manual is lower", ptr(5), ptr(14), sendlimit.Value{Limit: ptr(5), Source: sendlimit.SourceManual}},
		{"provider is lower", ptr(50), ptr(14), sendlimit.Value{Limit: ptr(14), Source: sendlimit.SourceProvider}},
		{"manual wins a tie", ptr(14), ptr(14), sendlimit.Value{Limit: ptr(14), Source: sendlimit.SourceManual}},
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
	assert.True(t, sendlimit.EffectiveOf(&ent.Integration{ProviderQuotaUnavailable: true}).ProviderQuotaUnavailable)
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
