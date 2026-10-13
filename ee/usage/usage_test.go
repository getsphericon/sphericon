package usage_test

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func licenseFor(t *testing.T, features ...licensekey.Feature) *licensekey.License {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, nil, features...)
	require.NoError(t, err)
	lic, err := licensekey.Parse(key, pub, time.Now())
	require.NoError(t, err)
	return lic
}

func at(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, time.UTC) }

var (
	jan = at(2027, time.January, 1, 0)
	feb = at(2027, time.February, 1, 0)
	mar = at(2027, time.March, 1, 0)
)

// message records one Outbound message of Acme, as the send path would leave it.
func message(t *testing.T, env *testhelper.TestEnv, key string, kind outboundmessage.Kind, status outboundmessage.Status, when time.Time) {
	t.Helper()
	c := env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Create().
		SetKind(kind).
		SetIdempotencyKey("usage-test:" + key).
		SetStatus(status).
		SetClaimedAt(when)
	if status == outboundmessage.StatusSent {
		c.SetSentAt(when)
	}
	require.NoError(t, c.Exec(t.Context()))
}

// figure is Acme's emails_sent figure of the period, failing when there is none.
func figure(t *testing.T, env *testhelper.TestEnv, period time.Time) usage.Figure {
	t.Helper()
	return figureOf(t, env, period, usage.EmailsSent)
}

// figureOf is Acme's figure of the metric for the period, failing when there is none.
func figureOf(t *testing.T, env *testhelper.TestEnv, period time.Time, metric usage.Metric) usage.Figure {
	t.Helper()
	page, err := usage.List(t.Context(), env.DB, period, pagination.Params{Page: 1, PageSize: pagination.MaxPageSize})
	require.NoError(t, err)
	for _, w := range page.Items {
		if w.WorkspaceID != fixtures.AcmeID {
			continue
		}
		for _, f := range w.Figures {
			if f.Metric == metric {
				return f
			}
		}
	}
	require.Failf(t, "no figure", "Acme has no %s figure for %s", metric, period.Format("2006-01"))
	return usage.Figure{}
}

func TestThePeriodIsTheCalendarMonthInUTC(t *testing.T) {
	zone := time.FixedZone("far east", 14*3600)
	got := usage.PeriodOf(time.Date(2027, time.February, 1, 5, 0, 0, 0, zone))

	assert.Equal(t, at(2027, time.January, 1, 0), got, "05:00 on 1 Feb at +14:00 is still January in UTC")
	assert.Equal(t, feb, usage.PeriodOf(at(2027, time.February, 28, 23)))
}

func TestEmailsSentCountsOnePerRecipientOverAllSurfacesAndNeverASkippedOrFailedOne(t *testing.T) {
	env := testhelper.Setup(t)
	message(t, env, "b", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.January, 3, 10))
	message(t, env, "a", outboundmessage.KindAutomation, outboundmessage.StatusSent, at(2027, time.January, 4, 10))
	message(t, env, "t", outboundmessage.KindTransactional, outboundmessage.StatusSent, at(2027, time.January, 31, 23))
	message(t, env, "skipped", outboundmessage.KindBroadcast, outboundmessage.StatusSkipped, at(2027, time.January, 5, 10))
	message(t, env, "failed", outboundmessage.KindBroadcast, outboundmessage.StatusFailed, at(2027, time.January, 6, 10))
	message(t, env, "feb", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.February, 1, 0))

	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureOperator), feb.Add(time.Hour)))

	f := figure(t, env, jan)
	assert.EqualValues(t, 3, f.Value)
	assert.True(t, f.Final)
}

func TestLiveUsageReportsSendsStillInFlightApartFromEmailsSent(t *testing.T) {
	env := testhelper.Setup(t)
	now := at(2027, time.February, 10, 12)
	message(t, env, "sent", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.February, 2, 9))
	message(t, env, "in-flight", outboundmessage.KindBroadcast, outboundmessage.StatusPending, at(2027, time.February, 10, 11))

	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureOperator), now))

	f := figure(t, env, feb)
	assert.EqualValues(t, 1, f.Value, "an in-flight send is not yet sent")
	assert.False(t, f.Final)
	assert.True(t, f.ComputedAt.Equal(now), "computed_at is the time of the refresh")
	inFlight := figureOf(t, env, feb, usage.InFlightSends)
	assert.EqualValues(t, 1, inFlight.Value)
	assert.False(t, inFlight.Final)
}

func TestLiveUsageIsRefreshedAndNeverRecountedOnRead(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureOperator)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, at(2027, time.February, 10, 12)))
	message(t, env, "late", outboundmessage.KindTransactional, outboundmessage.StatusSent, at(2027, time.February, 10, 13))

	assert.EqualValues(t, 0, figure(t, env, feb).Value, "a read does not recount")

	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, at(2027, time.February, 10, 14)))
	assert.EqualValues(t, 1, figure(t, env, feb).Value)
}

func TestAClosedSnapshotNeverChangesAndARerunIsTheSame(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureOperator)
	message(t, env, "one", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.January, 3, 10))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(time.Minute)))
	closed := figure(t, env, jan)

	// A message dated inside January shows up after the close, and the job runs again.
	message(t, env, "straggler", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.January, 20, 10))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(time.Hour)))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(2*time.Hour)))

	assert.Equal(t, closed.Value, figure(t, env, jan).Value)
	assert.EqualValues(t, 1, figure(t, env, jan).Value)
	assert.True(t, closed.ComputedAt.Equal(figure(t, env, jan).ComputedAt))
}

func TestALateRunClosesEveryElapsedPeriodWithTheSameFiguresAsAnOnTimeRun(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureOperator)
	message(t, env, "jan", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.January, 3, 10))
	message(t, env, "feb1", outboundmessage.KindAutomation, outboundmessage.StatusSent, at(2027, time.February, 3, 10))
	message(t, env, "feb2", outboundmessage.KindAutomation, outboundmessage.StatusSent, at(2027, time.February, 4, 10))
	message(t, env, "mar", outboundmessage.KindTransactional, outboundmessage.StatusSent, at(2027, time.March, 4, 10))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, jan.Add(24*time.Hour)), "an early run leaves January open")

	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, at(2027, time.April, 9, 0)))

	for period, want := range map[time.Time]int64{jan: 1, feb: 2, mar: 1} {
		f := figure(t, env, period)
		assert.Equal(t, want, f.Value, period.Format("2006-01"))
		assert.True(t, f.Final, period.Format("2006-01"))
	}
}

func TestAMessageInFlightAtTheCloseIsCountedInThePeriodItWasSentIn(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureOperator)
	claimed := at(2027, time.January, 31, 23)
	message(t, env, "in-flight", outboundmessage.KindBroadcast, outboundmessage.StatusPending, claimed)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(time.Minute)))
	assert.EqualValues(t, 0, figure(t, env, jan).Value, "a send not yet accepted is not in a snapshot")

	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).OutboundMessage().Update().
		Where(outboundmessage.IdempotencyKey("usage-test:in-flight")).
		SetStatus(outboundmessage.StatusSent).SetSentAt(feb.Add(2*time.Minute)).Exec(t.Context()))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, feb.Add(time.Hour)))

	assert.EqualValues(t, 0, figure(t, env, jan).Value)
	assert.EqualValues(t, 1, figure(t, env, feb).Value)
}

func TestNothingIsMaterializedWithoutTheOperatorLicense(t *testing.T) {
	env := testhelper.Setup(t)
	message(t, env, "one", outboundmessage.KindBroadcast, outboundmessage.StatusSent, at(2027, time.January, 3, 10))

	for _, lic := range []*licensekey.License{nil, licenseFor(t), licenseFor(t, licensekey.FeatureAudit)} {
		require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, mar))
	}

	page, err := usage.List(t.Context(), env.DB, jan, pagination.Params{Page: 1, PageSize: 25})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestListPagesEveryWorkspaceByIdAndDefaultsNothingItselfToARowlessPeriod(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureOperator), feb.Add(time.Hour)))

	first, err := usage.List(t.Context(), env.DB, feb, pagination.Params{Page: 1, PageSize: 1})
	require.NoError(t, err)
	second, err := usage.List(t.Context(), env.DB, feb, pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)

	require.Len(t, first.Items, 1)
	require.Len(t, second.Items, 1)
	assert.Less(t, first.Items[0].WorkspaceID, second.Items[0].WorkspaceID)
	assert.GreaterOrEqual(t, first.TotalItems, 4, "every Workspace has a figure")
	assert.Equal(t, feb, first.Items[0].Period)
}
