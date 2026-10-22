package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Broadcast and Automation reports show the delivery result stamped on the Outbound messages
// (ADR 0053), so an old report does not shrink when its analytical Events expire. `delayed` and
// `dropped` are on the timeline only: the reports have no field for them.

// pruneAllAnalytical expires every analytical Event, as retention does once they are old enough.
func pruneAllAnalytical(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	_, err := events.PruneEvents(t.Context(), env.DB, time.Nanosecond, 1000)
	require.NoError(t, err)
}

func broadcastStats(t *testing.T, env *testhelper.TestEnv) siteapi.SiteBroadcastStats {
	t.Helper()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteBroadcastsGet(t.Context(), siteapi.SiteBroadcastsGetParams{
		WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID),
		ID:          siteapi.EntityId(fixtures.BroadcastSentID),
	})
	require.NoError(t, err)
	b, ok := res.(*siteapi.SiteBroadcastResource)
	require.True(t, ok, "got %T", res)
	return b.Stats
}

func TestBroadcastReportCountsTheDeliveryResultAndSurvivesEventPruning(t *testing.T) {
	env := testhelper.Setup(t)

	before := broadcastStats(t, env)
	assert.EqualValues(t, 3, before.DeliveredCount)
	assert.EqualValues(t, 1, before.BouncedCount)
	assert.EqualValues(t, 1, before.ComplainedCount)

	pruneAllAnalytical(t, env)

	assert.Equal(t, before, broadcastStats(t, env))
}

func TestExternalBroadcastReportCountsTheDeliveryResult(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "broadcasts:read")

	pruneAllAnalytical(t, env)

	res, err := c.BroadcastsReport(t.Context(), externalapi.BroadcastsReportParams{ID: fixtures.BroadcastSentID})
	require.NoError(t, err)
	r, ok := res.(*externalapi.BroadcastReport)
	require.True(t, ok, "got %T", res)
	assert.EqualValues(t, 3, r.DeliveredCount)
	assert.EqualValues(t, 1, r.BouncedCount)
	assert.EqualValues(t, 1, r.ComplainedCount)
}

func TestAutomationReportCountsTheDeliveryResultAndSurvivesEventPruning(t *testing.T) {
	env := testhelper.Setup(t)
	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageAutomationTrackedID))
	persistEngagement(t, env)

	before := automationReport(t, env, fixtures.AutomationWelcomeSeriesID)
	assert.EqualValues(t, 1, before.Opens)
	// Two messages of the Automation: both delivered, one bounced, one complained. The other
	// Automation's message is not its figure.
	assert.EqualValues(t, 2, before.Delivered)
	assert.EqualValues(t, 1, before.Bounced)
	assert.EqualValues(t, 1, before.Complained)

	pruneAllAnalytical(t, env)

	after := automationReport(t, env, fixtures.AutomationWelcomeSeriesID)
	assert.Zero(t, after.Opens, "engagement counts come from Events and expire with them")
	assert.Equal(t, before.Delivered, after.Delivered)
	assert.Equal(t, before.Bounced, after.Bounced)
	assert.Equal(t, before.Complained, after.Complained)
}
