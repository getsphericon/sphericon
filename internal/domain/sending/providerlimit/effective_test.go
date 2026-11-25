package providerlimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// reads are the limits read at the wall clock.
var reads = providerlimit.New(time.Now)

func TestEffectiveOfReportsManualValuesAndTheirSource(t *testing.T) {
	perSecond, perDay := 14, 50000

	none := providerlimit.EffectiveOf(integ(ent.Integration{}, ent.IntegrationProviderState{}))
	assert.True(t, none.Unlimited())
	assert.Nil(t, none.PerSecond.Limit)
	assert.Empty(t, none.PerSecond.Source, "no ceiling has no source")

	both := providerlimit.EffectiveOf(integ(ent.Integration{MaxPerSecond: &perSecond, MaxPerDay: &perDay}, ent.IntegrationProviderState{}))
	assert.False(t, both.Unlimited())
	assert.Equal(t, providerlimit.Value{Limit: &perSecond, Source: providerlimit.SourceManual}, both.PerSecond)
	assert.Equal(t, providerlimit.Value{Limit: &perDay, Source: providerlimit.SourceManual}, both.PerDay)
	assert.Equal(t, providerlimit.Limits{PerSecond: &perSecond, PerDay: &perDay}, both.Limits())

	onlyDay := providerlimit.EffectiveOf(integ(ent.Integration{MaxPerDay: &perDay}, ent.IntegrationProviderState{}))
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
			got := providerlimit.EffectiveOf(integ(ent.Integration{MaxPerSecond: c.manual, MaxPerDay: c.manual},
				ent.IntegrationProviderState{ProviderMaxPerSecond: c.provider, ProviderMaxPerDay: c.provider}))
			assert.Equal(t, c.want, got.PerSecond)
			assert.Equal(t, c.want, got.PerDay)
		})
	}
}

func TestEffectiveOfReportsAnUnavailableProviderQuota(t *testing.T) {
	assert.False(t, providerlimit.EffectiveOf(integ(ent.Integration{}, ent.IntegrationProviderState{})).ProviderQuotaUnavailable)
	assert.True(t, providerlimit.EffectiveOf(integ(ent.Integration{}, ent.IntegrationProviderState{ProviderQuotaFailedAt: new(time.Now())})).ProviderQuotaUnavailable)
}

func TestEffectiveOfReportsTheAccountStanding(t *testing.T) {
	assert.Equal(t, providers.EnforcementNone, providerlimit.EffectiveOf(integ(ent.Integration{}, ent.IntegrationProviderState{})).Enforcement)
	stopped := providers.EnforcementShutdown
	eff := providerlimit.EffectiveOf(integ(ent.Integration{}, ent.IntegrationProviderState{ProviderEnforcement: &stopped, ProviderSandbox: true}))
	assert.Equal(t, providers.EnforcementShutdown, eff.Enforcement)
	assert.True(t, eff.Sandbox)
}

func TestEffectiveOfWarnsOfNoCeilingExceptOnATestInbox(t *testing.T) {
	smtp := providerlimit.EffectiveOf(integ(ent.Integration{Provider: providers.SMTP}, ent.IntegrationProviderState{}))
	inbox := providerlimit.EffectiveOf(integ(ent.Integration{Provider: providers.Inbox}, ent.IntegrationProviderState{}))

	assert.Equal(t, []providerlimit.Warning{providerlimit.WarningUnlimited}, smtp.Warnings())
	assert.Empty(t, inbox.Warnings())
}

func TestStoppedIsTheShutdownAlone(t *testing.T) {
	stopped, probation := providers.EnforcementShutdown, providers.EnforcementProbation
	assert.True(t, providerlimit.Stopped(integ(ent.Integration{}, ent.IntegrationProviderState{ProviderEnforcement: &stopped})))
	assert.False(t, providerlimit.Stopped(integ(ent.Integration{}, ent.IntegrationProviderState{ProviderEnforcement: &probation})), "probation still sends")
	assert.False(t, providerlimit.Stopped(integ(ent.Integration{}, ent.IntegrationProviderState{ProviderSandbox: true})), "so does the sandbox")
	assert.False(t, providerlimit.Stopped(integ(ent.Integration{}, ent.IntegrationProviderState{})))
}

func TestUsageCountsSentMessagesPerIntegrationOverTheLastDay(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	got, err := reads.Usage(ctx, env.DB.Scoped(fixtures.AcmeID))
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
	rows, err := s.Integration().Query().WithProviderState().All(ctx)
	require.NoError(t, err)
	usage, err := reads.Usage(ctx, s)
	require.NoError(t, err)

	got, err := reads.Statuses(ctx, s, rows)
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

	usage, usageErr := reads.Usage(ctx, s)
	statuses, statusesErr := reads.Statuses(ctx, s, rows)

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

	inside, err := providerlimit.New(func() time.Time { return oldest.SentAt.Add(23*time.Hour + 30*time.Minute) }).Usage(ctx, s)
	require.NoError(t, err)
	outside, err := providerlimit.New(func() time.Time { return oldest.SentAt.Add(24*time.Hour + 30*time.Minute) }).Usage(ctx, s)
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
		tokens     float64
		refilledAt time.Time
		n          int
		want       time.Duration
	}{
		"a full bucket within the cap":          {100, now, 100, 0},
		"a full bucket beyond the cap":          {100, now, 150, 12 * time.Hour},
		"a quarter refilled for three hours":    {25, now.Add(-3 * time.Hour), 50, 3 * time.Hour},
		"a bucket in debt from Transactionals":  {-50, now, 50, 24 * time.Hour},
		"a bucket that refilled to full and on": {0, now.Add(-48 * time.Hour), 100, 0},
	} {
		require.NoError(t, acme.SendCapLimiter().UpdateOneID(fixtures.SendCapLimiterAcmeID).
			SetDayTokens(tc.tokens).SetRefilledAt(tc.refilledAt).Exec(ctx))

		got, err := reads.SendCapDrain(ctx, acme, providerlimit.DrainInput{SendCap: 100, N: tc.n, At: now})

		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}

	tyrell := env.DB.Scoped(fixtures.TyrellID)
	within, err := reads.SendCapDrain(ctx, tyrell, providerlimit.DrainInput{SendCap: 100, N: 100, At: now})
	require.NoError(t, err)
	beyond, err := reads.SendCapDrain(ctx, tyrell, providerlimit.DrainInput{SendCap: 100, N: 150, At: now})
	require.NoError(t, err)
	assert.Zero(t, within, "a Workspace without a bucket row starts full")
	assert.Equal(t, 12*time.Hour, beyond)
}

func TestSendCapDrainReturnsTheStoresErrorAndNoWait(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	got, err := reads.SendCapDrain(ctx, env.DB.Scoped(fixtures.AcmeID), providerlimit.DrainInput{SendCap: 100, N: 150, At: time.Now()})

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, got)
}

// integ is row read with state as its provider state (ADR 0168).
func integ(row ent.Integration, state ent.IntegrationProviderState) *ent.Integration {
	row.Edges.ProviderState = &state
	return &row
}
