package usage_test

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
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
)

// month is the first instant of the calendar month n months back from now, the way the Meter
// fixtures' monthAgo places their rows; moment is a day and hour inside it.
func month(n int) time.Time { return usage.PeriodOf(time.Now()).AddDate(0, -n, 0) }

func moment(n, day, hour int) time.Time {
	return month(n).AddDate(0, 0, day-1).Add(time.Duration(hour) * time.Hour)
}

// settle moves a pending fixture message of Meter to sent at `when`: the send arrives after the
// moment a test first measured.
func settle(t *testing.T, env *testhelper.TestEnv, key string, when time.Time) {
	t.Helper()
	require.NoError(t, env.DB.Scoped(fixtures.MeterID).OutboundMessage().Update().
		Where(outboundmessage.IdempotencyKey("meter:"+key)).
		SetStatus(outboundmessage.StatusSent).SetSentAt(when).Exec(t.Context()))
}

// figure is Meter's emails_sent figure of the period, failing when there is none.
func figure(t *testing.T, env *testhelper.TestEnv, period time.Time) usage.Figure {
	t.Helper()
	return figureFor(t, env, fixtures.MeterID, period, usage.EmailsSent)
}

// figureOf is Acme's figure of the metric for the period, failing when there is none.
func figureOf(t *testing.T, env *testhelper.TestEnv, period time.Time, metric usage.Metric) usage.Figure {
	t.Helper()
	return figureFor(t, env, fixtures.AcmeID, period, metric)
}

func figureFor(t *testing.T, env *testhelper.TestEnv, workspaceID int64, period time.Time, metric usage.Metric) usage.Figure {
	t.Helper()
	page, err := usage.List(t.Context(), env.DB, period, pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, err)
	for _, w := range page.Items {
		if w.WorkspaceID != workspaceID {
			continue
		}
		for _, f := range w.Figures {
			if f.Metric == metric {
				return f
			}
		}
	}
	require.Failf(t, "no figure", "workspace %d has no %s figure for %s", workspaceID, metric, period.Format("2006-01"))
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
	// Meter's month 17 holds three sends (a Broadcast, an Automation, a Transactional), one skipped
	// and one failed; the first instant of the next month holds a fourth.
	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), month(16).Add(time.Hour)))

	f := figure(t, env, month(17))
	assert.EqualValues(t, 3, f.Value)
	assert.True(t, f.Final)
}

func TestLiveUsageReportsSendsStillInFlightApartFromEmailsSent(t *testing.T) {
	env := testhelper.Setup(t)
	now := moment(14, 10, 12)

	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), now))

	f := figure(t, env, month(14))
	assert.EqualValues(t, 1, f.Value, "an in-flight send is not yet sent")
	assert.False(t, f.Final)
	assert.True(t, f.ComputedAt.Equal(now), "computed_at is the time of the refresh")
	inFlight := figureFor(t, env, fixtures.MeterID, month(14), usage.InFlightSends)
	assert.EqualValues(t, 1, inFlight.Value)
	assert.False(t, inFlight.Final)
}

func TestLiveUsageIsRefreshedAndNeverRecountedOnRead(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureStaff)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, moment(12, 10, 12)))
	settle(t, env, "late", moment(12, 10, 13))

	assert.EqualValues(t, 0, figure(t, env, month(12)).Value, "a read does not recount")

	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, moment(12, 10, 14)))
	assert.EqualValues(t, 1, figure(t, env, month(12)).Value)
}

func TestAClosedSnapshotNeverChangesAndARerunIsTheSame(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureStaff)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(9).Add(time.Minute)))
	closed := figure(t, env, month(10))

	// A message dated inside the closed month is accepted after the close, and the job runs again.
	settle(t, env, "closed-straggler", moment(10, 20, 10))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(9).Add(time.Hour)))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(9).Add(2*time.Hour)))

	assert.Equal(t, closed.Value, figure(t, env, month(10)).Value)
	assert.EqualValues(t, 1, figure(t, env, month(10)).Value)
	assert.True(t, closed.ComputedAt.Equal(figure(t, env, month(10)).ComputedAt))
}

func TestALateRunClosesEveryElapsedPeriodWithTheSameFiguresAsAnOnTimeRun(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureStaff)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(7).Add(24*time.Hour)), "an early run leaves the first month open")

	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, moment(4, 9, 0)))

	for period, want := range map[time.Time]int64{month(7): 1, month(6): 2, month(5): 1} {
		f := figure(t, env, period)
		assert.Equal(t, want, f.Value, period.Format("2006-01"))
		assert.True(t, f.Final, period.Format("2006-01"))
	}
}

func TestAMessageInFlightAtTheCloseIsCountedInThePeriodItWasSentIn(t *testing.T) {
	env := testhelper.Setup(t)
	lic := licenseFor(t, licensekey.FeatureStaff)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(2).Add(time.Minute)))
	assert.EqualValues(t, 0, figure(t, env, month(3)).Value, "a send not yet accepted is not in a snapshot")

	settle(t, env, "close-inflight", month(2).Add(2*time.Minute))
	require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(2).Add(time.Hour)))

	assert.EqualValues(t, 0, figure(t, env, month(3)).Value)
	assert.EqualValues(t, 1, figure(t, env, month(2)).Value)
}

func TestNothingIsMaterializedWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t)
	for _, lic := range []*licensekey.License{nil, licenseFor(t), licenseFor(t, licensekey.FeatureAudit)} {
		require.NoError(t, usage.Refresh(t.Context(), env.DB, lic, month(5)))
	}

	page, err := usage.List(t.Context(), env.DB, month(7), pagination.Params{Page: 1, PageSize: 25})
	require.NoError(t, err)
	assert.Empty(t, page.Items)
}

func TestListPagesEveryWorkspaceByIdAndDefaultsNothingItselfToARowlessPeriod(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), month(6).Add(time.Hour)))

	first, err := usage.List(t.Context(), env.DB, month(6), pagination.Params{Page: 1, PageSize: 1})
	require.NoError(t, err)
	second, err := usage.List(t.Context(), env.DB, month(6), pagination.Params{Page: 2, PageSize: 1})
	require.NoError(t, err)

	require.Len(t, first.Items, 1)
	require.Len(t, second.Items, 1)
	assert.Less(t, first.Items[0].WorkspaceID, second.Items[0].WorkspaceID)
	assert.GreaterOrEqual(t, first.TotalItems, 4, "every Workspace has a figure")
	assert.Equal(t, month(6), first.Items[0].Period)
}

func TestOneFailingWorkspaceDoesNotStopTheOthersFromClosing(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("boom")
	env.DB.Usage.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if id, ok := m.(interface{ WorkspaceID() (int64, bool) }); ok {
				if ws, set := id.WorkspaceID(); set && ws == fixtures.AcmeID {
					return nil, boom
				}
			}
			return next.Mutate(ctx, m)
		})
	})

	err := usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), month(6).Add(time.Hour))

	require.ErrorIs(t, err, boom, "the failure is still reported")
	page, listErr := usage.List(t.Context(), env.DB, month(6), pagination.Params{Page: 1, PageSize: 100})
	require.NoError(t, listErr)
	assert.NotEmpty(t, page.Items, "Workspaces after the failing one are still refreshed")
	for _, w := range page.Items {
		assert.NotEqual(t, int64(fixtures.AcmeID), w.WorkspaceID)
	}
}

func TestRefreshSkipsAWorkspaceInItsDeletionWindow(t *testing.T) {
	env := testhelper.Setup(t)
	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(month(7)).ExecX(t.Context())

	require.NoError(t, usage.Refresh(t.Context(), env.DB, licenseFor(t, licensekey.FeatureStaff), month(6).Add(time.Hour)))

	page, err := usage.List(t.Context(), env.DB, month(7), pagination.Params{Page: 1, PageSize: 25})
	require.NoError(t, err)
	ids := make([]int64, 0, len(page.Items))
	for _, w := range page.Items {
		ids = append(ids, w.WorkspaceID)
	}
	assert.NotEmpty(t, ids, "live Workspaces are still measured")
	assert.NotContains(t, ids, fixtures.AcmeID)
}
