package providerlimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
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

func TestEffectiveOfReportsTheAccountStanding(t *testing.T) {
	assert.Equal(t, providers.EnforcementNone, providerlimit.EffectiveOf(&ent.Integration{}).Enforcement)
	stopped := providers.EnforcementShutdown
	eff := providerlimit.EffectiveOf(&ent.Integration{ProviderEnforcement: &stopped, ProviderSandbox: true})
	assert.Equal(t, providers.EnforcementShutdown, eff.Enforcement)
	assert.True(t, eff.Sandbox)
}

func TestEffectiveOfWarnsOfNoCeilingExceptOnATestInbox(t *testing.T) {
	smtp := providerlimit.EffectiveOf(&ent.Integration{Provider: providers.SMTP})
	inbox := providerlimit.EffectiveOf(&ent.Integration{Provider: providers.Inbox})

	assert.Equal(t, []providerlimit.Warning{providerlimit.WarningUnlimited}, smtp.Warnings())
	assert.Empty(t, inbox.Warnings())
}

func TestStoppedIsTheShutdownAlone(t *testing.T) {
	stopped, probation := providers.EnforcementShutdown, providers.EnforcementProbation
	assert.True(t, providerlimit.Stopped(&ent.Integration{ProviderEnforcement: &stopped}))
	assert.False(t, providerlimit.Stopped(&ent.Integration{ProviderEnforcement: &probation}), "probation still sends")
	assert.False(t, providerlimit.Stopped(&ent.Integration{ProviderSandbox: true}), "so does the sandbox")
	assert.False(t, providerlimit.Stopped(&ent.Integration{}))
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
		{"no ceiling on a Test inbox", providerlimit.Effective{Inbox: true}, []providerlimit.Warning{}},
		{"provider quota unreadable", providerlimit.Effective{PerDay: limited, ProviderQuotaUnavailable: true}, []providerlimit.Warning{providerlimit.WarningProviderQuotaUnavailable}},
		{"both", providerlimit.Effective{ProviderQuotaUnavailable: true}, []providerlimit.Warning{providerlimit.WarningUnlimited, providerlimit.WarningProviderQuotaUnavailable}},
		{"account stopped", providerlimit.Effective{PerSecond: limited, Enforcement: providers.EnforcementShutdown}, []providerlimit.Warning{providerlimit.WarningProviderStopped}},
		{"account on probation", providerlimit.Effective{PerSecond: limited, Enforcement: providers.EnforcementProbation}, []providerlimit.Warning{providerlimit.WarningProviderProbation}},
		{"account in the sandbox", providerlimit.Effective{PerSecond: limited, Sandbox: true}, []providerlimit.Warning{providerlimit.WarningProviderSandbox}},
		{"all", providerlimit.Effective{ProviderQuotaUnavailable: true, Enforcement: providers.EnforcementShutdown, Sandbox: true}, []providerlimit.Warning{
			providerlimit.WarningUnlimited, providerlimit.WarningProviderQuotaUnavailable, providerlimit.WarningProviderStopped, providerlimit.WarningProviderSandbox,
		}},
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

	require.NoError(t, s.Integration().UpdateOneID(fixtures.IntegrationAcmeSesID).SetMaxPerSecond(5).SetMaxPerDay(1000).Exec(ctx))
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
	ses := got[fixtures.IntegrationAcmeSesID]
	assert.Equal(t, providerlimit.Value{Limit: new(5), Source: providerlimit.SourceManual}, ses.PerSecond)
	assert.Equal(t, providerlimit.Value{Limit: new(1000), Source: providerlimit.SourceManual}, ses.PerDay)
}

func TestUsageAndStatusesReturnTheStoresError(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	rows, err := s.Integration().Query().All(t.Context())
	require.NoError(t, err)

	usage, usageErr := providerlimit.Usage(ctx, s, time.Now())
	statuses, statusesErr := providerlimit.Statuses(ctx, s, rows, time.Now())

	require.ErrorIs(t, usageErr, context.Canceled)
	assert.Nil(t, usage)
	require.ErrorIs(t, statusesErr, context.Canceled)
	assert.Nil(t, statuses)
}

// The window is the 24 hours before now: the default Integration's oldest send counts until 24 hours
// after it went out and no longer after.
func TestUsageCountsOverTheLast24HoursExactly(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()
	oldest, err := s.OutboundMessage().Query().
		Where(
			outboundmessage.IntegrationIDEQ(fixtures.IntegrationAcmeDefaultID),
			outboundmessage.StatusEQ(outboundmessage.StatusSent),
			outboundmessage.SentAtNotNil(),
		).
		Order(ent.Asc(outboundmessage.FieldSentAt)).
		First(ctx)
	require.NoError(t, err)

	inside, err := providerlimit.Usage(ctx, s, oldest.SentAt.Add(23*time.Hour+30*time.Minute))
	require.NoError(t, err)
	outside, err := providerlimit.Usage(ctx, s, oldest.SentAt.Add(24*time.Hour+30*time.Minute))
	require.NoError(t, err)

	countedInside, countedOutside := inside[fixtures.IntegrationAcmeDefaultID], outside[fixtures.IntegrationAcmeDefaultID]
	assert.Equal(t, countedOutside+1, countedInside, "the oldest send drops out once it is more than 24 hours old")
}

// A Send cap of 100 a day: a full bucket lets 100 messages out at once, and each one beyond what the
// bucket holds takes 864 seconds (a day over 100), longer from a bucket in debt. The refill since
// the bucket was last written counts, and a Workspace that never sent under its Send cap has a full
// bucket.
func TestSendCapDrainIsTheTimeTheBucketTakesToHoldTheMessages(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	now := time.Now().Truncate(time.Second)
	acme := env.DB.Scoped(fixtures.AcmeID)

	for name, tc := range map[string]struct {
		fill       float64
		refilledAt time.Time
		n          int
		want       time.Duration
	}{
		"a full bucket within the cap":          {1, now, 100, 0},
		"a full bucket beyond the cap":          {1, now, 150, 12 * time.Hour},
		"a quarter refilled for three hours":    {0.25, now.Add(-3 * time.Hour), 50, 3 * time.Hour},
		"a bucket in debt from Transactionals":  {-0.5, now, 50, 24 * time.Hour},
		"a bucket that refilled to full and on": {0, now.Add(-48 * time.Hour), 100, 0},
	} {
		require.NoError(t, acme.SendCapLimiter().UpdateOneID(fixtures.SendCapLimiterAcmeID).
			SetDayFill(tc.fill).SetRefilledAt(tc.refilledAt).Exec(ctx))

		got, err := providerlimit.SendCapDrain(ctx, acme, 100, tc.n, now)

		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}

	tyrell := env.DB.Scoped(fixtures.TyrellID)
	within, err := providerlimit.SendCapDrain(ctx, tyrell, 100, 100, now)
	require.NoError(t, err)
	beyond, err := providerlimit.SendCapDrain(ctx, tyrell, 100, 150, now)
	require.NoError(t, err)
	assert.Zero(t, within, "a Workspace without a bucket row starts full")
	assert.Equal(t, 12*time.Hour, beyond)
}

func TestSendCapDrainReturnsTheStoresErrorAndNoWait(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := providerlimit.SendCapDrain(ctx, env.DB.Scoped(fixtures.AcmeID), 100, 150, time.Now())

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, got)
}
