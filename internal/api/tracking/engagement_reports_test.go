package tracking_test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The reports count tracked messages only and the untracked ones beside them (ADR 0120): a message
// sent to a Destination that refused tracking leaves the Engagement rates' denominator and a
// Broadcast's opened and clicked figures, and is counted as untracked.

// globexUsageMessage is Globex's one Broadcast message in every analytics window, sent tracked
// (outbound_messages.yml).
const globexUsageMessage = 901

// globexOverview is Globex's analytics overview of the last 7 days, as its Owner reads it.
func globexOverview(t *testing.T, env *testhelper.TestEnv) *siteapi.SiteAnalyticsOverview {
	t.Helper()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	ov, err := jane.SiteAnalyticsOverview(t.Context(), siteapi.SiteAnalyticsOverviewParams{
		WorkspaceId: fixtures.GlobexPublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
	})
	require.NoError(t, err)
	return ov
}

// With one tracked and one untracked Broadcast message in the window, the rates are over the
// tracked one and the untracked one is counted beside them, in the window and in its Report day.
func TestTheEngagementRatesCountTrackedMessagesAndTheUntrackedOnesBeside(t *testing.T) {
	env := testhelper.Setup(t)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)
	sendToReader(t, env)
	trackingGet(t, env, openPath(t, env, globexUsageMessage))
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})

	ov := globexOverview(t, env)

	email := ov.Current.Email
	assert.Equal(t, int32(1), email.SentCount, "the denominator is the tracked message")
	assert.Equal(t, int32(1), email.UntrackedCount)
	assert.Equal(t, int32(1), email.OpenedCount)
	assert.InDelta(t, 1, email.OpenRate, 1e-6)
	assert.Equal(t, int32(2), ov.Current.Sending.Broadcast, "both messages were sent")
	untracked := lo.SumBy(ov.Timeseries, func(p siteapi.SiteAnalyticsPoint) int32 { return p.Untracked })
	assert.Equal(t, int32(1), untracked, "the series reconciles with the cards")
}

// A Broadcast's figures report its untracked count beside what it sent, on /site and on /api, and
// its open and click rates are over its tracked messages: none here.
func TestABroadcastsFiguresReportItsUntrackedCount(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)
	sendToReader(t, env)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})

	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	site, err := jane.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)
	api := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, apitokens.ScopeBroadcastsRead))
	report, err := api.BroadcastsReport(ctx, externalapi.BroadcastsReportParams{ID: externalapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)

	assert.Equal(t, int32(1), site.Stats.SentCount)
	assert.Equal(t, int32(1), site.Stats.UntrackedCount)
	assert.Zero(t, site.Stats.OpenRate.Value, "no tracked message to open")
	assert.Equal(t, int32(1), report.SentCount)
	assert.Equal(t, int32(1), report.UntrackedCount)
}

// Umbrella's tracking mix Broadcast (broadcasts.yml): of its three messages one is tracked, one was
// opened and clicked before its tracking was withdrawn and one went out untracked. Its opened and
// clicked are counted over the tracked one, and its rates over it too.
func TestABroadcastsOpenedAndClickedAreCountedOverItsTrackedMessages(t *testing.T) {
	env := testhelper.Setup(t)
	api := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.UmbrellaID, apitokens.ScopeBroadcastsRead))

	report, err := api.BroadcastsReport(t.Context(), externalapi.BroadcastsReportParams{ID: externalapi.EntityId(fixtures.BroadcastUmbrellaTrackingMixID)})
	require.NoError(t, err)

	assert.Equal(t, int32(3), report.SentCount)
	assert.Equal(t, int32(2), report.UntrackedCount)
	assert.Equal(t, int32(1), report.OpenedCount.Value)
	assert.Equal(t, int32(1), report.ClickedCount)
	assert.InDelta(t, 1, report.OpenRate.Value, 1e-6, "opened ÷ the tracked message")
	assert.InDelta(t, 1, report.ClickRate, 1e-6, "clicked ÷ the tracked message")
}
