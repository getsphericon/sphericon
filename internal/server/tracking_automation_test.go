package server_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Automation emails are tracked like a Broadcast's: the token names the Outbound message and the
// Event carries the Automation (ADR 0053).

func automationReport(t *testing.T, env *testhelper.TestEnv, automationID int64) *siteapi.SiteAutomationReport {
	t.Helper()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteAutomationReportsGet(t.Context(), siteapi.SiteAutomationReportsGetParams{
		WorkspaceId:  siteapi.WorkspaceId(fixtures.AcmePublicID),
		AutomationId: siteapi.EntityId(automationID),
	})
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAutomationReport)
	require.True(t, ok, "got %T", res)
	return report
}

func TestAutomationOpensAndClicksAreEventsCarryingTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, http.StatusOK, trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageAutomationTrackedID)).Code)
	link, err := env.Tracker.ClickURL(fixtures.OutboundMessageAutomationTrackedID, linkA)
	require.NoError(t, err)
	resp := trackingGet(t, env, "/e/c/"+link[len(env.Tracker.Origin()+"/e/c/"):])
	assert.Equal(t, http.StatusFound, resp.Code)
	assert.Equal(t, linkA, resp.Header().Get("Location"))

	for _, name := range []string{events.NameEmailOpened, events.NameEmailClicked} {
		got := env.OutboxEvents(t, name)
		require.Len(t, got, 1, name)
		e, ok := got[0].(*events.EmailEngagement)
		require.True(t, ok)
		assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, e.AutomationID)
		assert.EqualValues(t, fixtures.OutboundMessageAutomationTrackedID, e.OutboundMessageID)
		assert.Zero(t, e.BroadcastID, "an Automation email belongs to no Broadcast")
	}
}

func TestAutomationReportShowsOpensClicksAndTheLinkTable(t *testing.T) {
	env := testhelper.Setup(t)
	const id = fixtures.OutboundMessageAutomationTrackedID

	for range 2 {
		trackingGet(t, env, openPath(t, env, id))
	}
	for _, dest := range []string{linkA, linkA, linkB} {
		link, err := env.Tracker.ClickURL(id, dest)
		require.NoError(t, err)
		trackingGet(t, env, "/e/c/"+link[len(env.Tracker.Origin()+"/e/c/"):])
	}
	// The mail of another Automation is not this one's figure.
	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageAutomationOnOpenID))
	persistEngagement(t, env)

	report := automationReport(t, env, fixtures.AutomationWelcomeSeriesID)
	assert.EqualValues(t, 2, report.Opens)
	assert.EqualValues(t, 3, report.Clicks)
	links := map[string]int32{}
	for _, l := range report.Links {
		links[l.URL] = l.Clicks
	}
	assert.Equal(t, map[string]int32{linkA: 2, linkB: 1}, links)
}
