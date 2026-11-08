package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
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
	report := res
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
		got := env.QueuedEvents(t, name)
		require.Len(t, got, 1, name)
		e, ok := got[0].(*events.EmailEngagement)
		require.True(t, ok)
		assert.EqualValues(t, fixtures.AutomationWelcomeSeriesID, e.AutomationID)
		assert.EqualValues(t, fixtures.OutboundMessageAutomationTrackedID, e.OutboundMessageID)
		assert.Zero(t, e.BroadcastID, "an Automation email belongs to no Broadcast")
	}
}

// The first human open and click are stamped on the Automation's message (ADR 0053), once: a repeat
// leaves the stamp as it was, and a machine hit stamps nothing.
func TestAnAutomationMessageIsStampedOnceByAHumanHitAndNeverByAMachineOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	const id = fixtures.OutboundMessageAutomationTrackedID
	stamps := func() (opened, clicked *time.Time) {
		m := env.DB.OutboundMessage.GetX(ctx, id)
		return m.OpenedAt, m.ClickedAt
	}

	hit(t, env, openPath(t, env, id), scannerUA, personAddr)
	hit(t, env, clickPathTo(t, env, id, linkA), scannerUA, personAddr)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	opened, clicked := stamps()
	assert.Nil(t, opened, "a machine open stamps nothing")
	assert.Nil(t, clicked, "a machine click stamps nothing")

	hit(t, env, openPath(t, env, id), personUA, personAddr)
	hit(t, env, clickPathTo(t, env, id, linkA), personUA, personAddr)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	opened, clicked = stamps()
	require.NotNil(t, opened, "a person's open stamps the message")
	require.NotNil(t, clicked, "a person's click stamps the message")

	hit(t, env, openPath(t, env, id), personUA, personAddr)
	hit(t, env, clickPathTo(t, env, id, linkB), personUA, personAddr)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	again, clickedAgain := stamps()
	assert.True(t, opened.Equal(*again), "a second open leaves the stamp unchanged")
	assert.True(t, clicked.Equal(*clickedAgain), "a second click leaves the stamp unchanged")
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

	report := automationReport(t, env, fixtures.AutomationWelcomeSeriesID)
	assert.EqualValues(t, 2, report.Opens)
	assert.EqualValues(t, 3, report.Clicks)
	links := map[string]int32{}
	for _, l := range report.Links {
		links[l.URL] = l.Clicks
	}
	assert.Equal(t, map[string]int32{linkA: 2, linkB: 1}, links)
}
