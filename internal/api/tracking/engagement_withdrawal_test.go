package tracking_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A refusal reaches back (ADR 0120): once a Destination refuses tracking, the opens and clicks
// recorded of its earlier messages are gone from the reports and a Broadcast's figures, the
// messages count as untracked, and an Order credited through one of their clicks has no credit.

// readerOrders places an Order Completed of Globex's reader through /api, as a shop does, and works
// it into its Order.
func readerOrders(t *testing.T, env *testhelper.TestEnv, orderID string) {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, apitokens.ScopeEventsWrite))
	err := c.EventsCreate(t.Context(), &externalapi.RecordEventsInput{Events: []externalapi.EventInput{{
		SubjectId: "reader",
		Email:     externalapi.NewOptNilEmailAddress(fixtures.ContactGlobexReaderEmail),
		Action:    "Order Completed",
		Properties: externalapi.NewOptNilJsonObject(externalapi.JsonObject{
			"order_id": jx.Raw(`"` + orderID + `"`), "total": jx.Raw(`20`), "currency": jx.Raw(`"USD"`),
		}),
	}}})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.OrderEventArgs{})
}

// globexBroadcast is the Globex Broadcast's figures as its Owner reads them, once the stamps'
// projection ran.
func globexBroadcast(t *testing.T, env *testhelper.TestEnv) siteapi.SiteBroadcastStats {
	t.Helper()
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	b, err := jane.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)
	return b.Stats
}

// refuseReader records the reader's refusal of tracking through /api and works the reach-back it
// starts.
func refuseReader(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})
}

// An email opened and clicked, and an Order credited through its click, then a refusal: the open
// and the click leave the analytics report and the Broadcast's figures, the message counts as
// untracked, and the Order is credited to nothing.
func TestARefusalDeletesTheOpensClicksAndCreditOfTheDestinationsEarlierEmail(t *testing.T) {
	env := testhelper.Setup(t)
	html := sendToReader(t, env)
	trackingGet(t, env, capturedPixel.FindString(html))
	trackingGet(t, env, capturedClick.FindString(html))
	readerOrders(t, env, "R-1")
	before := globexBroadcast(t, env)
	overviewBefore := globexOverview(t, env).Current.Email
	require.Equal(t, int32(1), before.OpenedCount.Value, "the open was counted")
	require.Equal(t, int32(1), before.ClickedCount)
	require.Equal(t, int32(1), before.Revenue.Orders, "the Order was credited through the click")

	refuseReader(t, env)

	after := globexBroadcast(t, env)
	overviewAfter := globexOverview(t, env).Current.Email
	assert.Zero(t, after.OpenedCount.Value, "the open is gone")
	assert.Zero(t, after.ClickedCount, "the click is gone")
	assert.Equal(t, before.UntrackedCount+1, after.UntrackedCount, "the message counts as untracked")
	assert.Zero(t, after.Revenue.Orders, "the Order has no credit")
	assert.Equal(t, overviewBefore.OpenedCount-1, overviewAfter.OpenedCount, "the report lost the open")
	assert.Equal(t, overviewBefore.ClickedCount-1, overviewAfter.ClickedCount, "and the click")
	assert.Equal(t, overviewBefore.SentCount-1, overviewAfter.SentCount, "the message left the rates' cohort")
	assert.Equal(t, overviewBefore.UntrackedCount+1, overviewAfter.UntrackedCount, "and is counted beside it")
}

// The reach-back is idempotent: working it a second time, as a retry or a repeated refusal does,
// changes nothing more.
func TestARefusalsReachBackTwiceChangesNothingMore(t *testing.T) {
	env := testhelper.Setup(t)
	html := sendToReader(t, env)
	trackingGet(t, env, capturedPixel.FindString(html))
	trackingGet(t, env, capturedClick.FindString(html))
	readerOrders(t, env, "R-2")
	refuseReader(t, env)
	once := globexBroadcast(t, env)
	overviewOnce := globexOverview(t, env).Current.Email

	env.EnqueueJob(t, jobs.TrackingWithdrawalArgs{
		WorkspaceID: fixtures.GlobexID, Channel: channels.Email, Destination: fixtures.ContactGlobexReaderEmail,
	})
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})

	twice := globexBroadcast(t, env)
	overviewTwice := globexOverview(t, env).Current.Email
	assert.Equal(t, once, twice)
	assert.Equal(t, overviewOnce, overviewTwice)
}

// What the open did stays (ADR 0120): the Enrollment the open started in an Automation triggered by
// opens keeps its place in the Automation's results, while the open itself leaves the report.
func TestARefusalKeepsTheEnrollmentItsDestinationsOpenStarted(t *testing.T) {
	env := testhelper.Setup(t)
	const message = fixtures.OutboundMessageAutomationOnOpenID
	trackingGet(t, env, openPath(t, env, message))
	env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
	before := automationReport(t, env, fixtures.AutomationOnOpenID)
	require.EqualValues(t, 1, before.Opens, "the open was counted")
	require.EqualValues(t, 1, before.Results.Current.Entered, "the open enrolled the Contact")

	acme := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.AcmeID, apitokens.ScopeContactsWrite))
	_, err := acme.EngagementTrackingCreate(t.Context(), &externalapi.RecordEngagementTrackingInput{
		Channel: externalapi.EngagementTrackingChannelEmail, Destination: "alice.automation@example.com",
		State: externalapi.EngagementTrackingStateDenied,
	})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})

	after := automationReport(t, env, fixtures.AutomationOnOpenID)
	assert.Zero(t, after.Opens, "the open is gone")
	assert.Equal(t, before.Results.Current.Entered, after.Results.Current.Entered, "the Enrollment stays")
}

// A Broadcast frozen by message retention keeps its figures (ADR 0120, ADR 0143): they are
// aggregates written once, and a later refusal of a Destination it reached does not move them,
// though the reach-back withdraws that Destination's message still kept.
func TestARefusalLeavesAFrozenBroadcastsFiguresAsTheyAre(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	figures := func() siteapi.SiteBroadcastStats {
		b, err := jane.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexRecentSendsID)})
		require.NoError(t, err)
		return b.Stats
	}
	trackingGet(t, env, openPath(t, env, globexUsageMessage))
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	before := figures()
	untrackedBefore := globexOverview(t, env).Current.Email.UntrackedCount

	globexTracking(t, env, "usage@globex.test", externalapi.EngagementTrackingStateDenied)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})

	after := figures()
	untrackedAfter := globexOverview(t, env).Current.Email.UntrackedCount
	assert.Equal(t, before, after, "the frozen figures stay as they were")
	assert.Equal(t, untrackedBefore+1, untrackedAfter, "the message still kept was withdrawn")
}
