package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Machine engagement (ADR 0053): an open or click made by software is an Event flagged machine and
// counts nowhere. The hit's user-agent and address are read for the verdict and never kept.

const (
	personUA  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
	scannerUA = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

	personAddr      = "192.0.2.10"    // outside every listed range
	relayAddr       = "198.51.100.77" // inside the PrivateRelayRange fixture
	relayAddrV6     = "2001:db8:1::5" // inside the PrivateRelayRangeV6 fixture
	publishedRange  = "203.0.113.0/24"
	publishedAddr   = "203.0.113.9"
	publishedRowCSV = publishedRange + ",US,US-CA,Los Angeles,\n"
)

// hit is a GET from a client with this user-agent and address, behind the proxy that appends the
// connecting address to X-Forwarded-For.
func hit(t *testing.T, env *testhelper.TestEnv, path, userAgent, addr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Forwarded-For", addr)
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, req)
	return rec
}

func clickPathTo(t *testing.T, env *testhelper.TestEnv, messageID int64, dest string) string {
	t.Helper()
	link, err := env.Tracker.ClickURL(messageID, dest)
	require.NoError(t, err)
	return "/e/c/" + link[len(env.Tracker.Origin()+"/e/c/"):]
}

func onlyEngagement(t *testing.T, env *testhelper.TestEnv, name string) *events.EmailEngagement {
	t.Helper()
	got := env.OutboxEvents(t, name)
	require.Len(t, got, 1, name)
	e, ok := got[0].(*events.EmailEngagement)
	require.True(t, ok)
	return e
}

func TestAScannerOpenIsAMachineEventAndCountsNowhere(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	resp := hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), scannerUA, personAddr)
	assert.Equal(t, http.StatusOK, resp.Code, "the pixel is served all the same")

	e := onlyEngagement(t, env, events.NameEmailOpened)
	assert.True(t, e.Machine)
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUntrackedID)
	assert.Nil(t, rec.OpenedAt, "a machine open sets no rollup")
	assert.Zero(t, figures(t, env, fixtures.BroadcastSentUntrackedID).Opened)
}

func TestAScannerClickRedirectsAndCountsNowhere(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	resp := hit(t, env, clickPathTo(t, env, fixtures.OutboundMessageTrackingSampleID, linkA), scannerUA, personAddr)
	assert.Equal(t, http.StatusFound, resp.Code)
	assert.Equal(t, linkA, resp.Header().Get("Location"))

	e := onlyEngagement(t, env, events.NameEmailClicked)
	assert.True(t, e.Machine)
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUntrackedID)
	assert.Nil(t, rec.ClickedAt, "a machine click sets no rollup")
	assert.Zero(t, figures(t, env, fixtures.BroadcastSentUntrackedID).Clicked)

	persistEngagement(t, env)
	assert.Empty(t, linkClicks(t, env), "a machine click is no row of the link table")
}

func TestAHitFromAPrivateRelayRangeIsMachine(t *testing.T) {
	env := testhelper.Setup(t)

	for _, addr := range []string{relayAddr, relayAddrV6} {
		hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), personUA, addr)
	}

	opened := env.OutboxEvents(t, events.NameEmailOpened)
	require.Len(t, opened, 2)
	for _, o := range opened {
		assert.True(t, o.(*events.EmailEngagement).Machine)
	}
	assert.Zero(t, figures(t, env, fixtures.BroadcastSentUntrackedID).Opened)
}

func TestAPersonsHitIsUnaffected(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), personUA, personAddr)
	hit(t, env, clickPathTo(t, env, fixtures.OutboundMessageTrackingSampleID, linkA), personUA, personAddr)

	assert.False(t, onlyEngagement(t, env, events.NameEmailOpened).Machine)
	assert.False(t, onlyEngagement(t, env, events.NameEmailClicked).Machine)
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUntrackedID)
	assert.NotNil(t, rec.OpenedAt)
	assert.NotNil(t, rec.ClickedAt)
	got := figures(t, env, fixtures.BroadcastSentUntrackedID)
	assert.Equal(t, 1, got.Opened)
	assert.Equal(t, 1, got.Clicked)
}

func TestAMachineOpenOfAnAutomationEmailIsNoFigureOfTheReport(t *testing.T) {
	env := testhelper.Setup(t)
	const id = fixtures.OutboundMessageAutomationTrackedID

	hit(t, env, openPath(t, env, id), personUA, personAddr)
	hit(t, env, openPath(t, env, id), scannerUA, personAddr)
	hit(t, env, clickPathTo(t, env, id, linkA), personUA, personAddr)
	hit(t, env, clickPathTo(t, env, id, linkB), scannerUA, personAddr)
	persistEngagement(t, env)

	report := automationReport(t, env, fixtures.AutomationWelcomeSeriesID)
	assert.EqualValues(t, 1, report.Opens)
	assert.EqualValues(t, 1, report.Clicks)
	require.Len(t, report.Links, 1)
	assert.Equal(t, linkA, report.Links[0].URL)
}

func TestAMachineOpenOrClickIsNoMatchOfASegmentCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	const alice = `{"kind":"rule","field":"email","operator":"eq","value":"` + fixtures.ContactAliceEmail + `"}`
	matching := func(action string) int32 {
		t.Helper()
		def := `{"kind":"group","combinator":"and","rules":[` + alice +
			`,{"kind":"event","action":"` + action + `","operator":"atLeast","times":1}]}`
		out, err := c.SiteSegmentsPreview(t.Context(), &siteapi.SitePreviewSegmentInput{
			Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
		}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		res, ok := out.(*siteapi.SitePreviewSegmentResult)
		require.Truef(t, ok, "got %T", out)
		return res.Count
	}

	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), scannerUA, personAddr)
	hit(t, env, clickPathTo(t, env, fixtures.OutboundMessageTrackingSampleID, linkA), scannerUA, personAddr)
	persistEngagement(t, env)
	assert.Zero(t, matching(events.NameEmailOpened), "a machine open is no open")
	assert.Zero(t, matching(events.NameEmailClicked), "a machine click is no click")

	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), personUA, personAddr)
	hit(t, env, clickPathTo(t, env, fixtures.OutboundMessageTrackingSampleID, linkB), personUA, personAddr)
	persistEngagement(t, env)
	assert.EqualValues(t, 1, matching(events.NameEmailOpened))
	assert.EqualValues(t, 1, matching(events.NameEmailClicked))
}

func TestTheEventListLeavesOutMachineEventsUnlessAskedFor(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	list := func(includeMachine bool) *siteapi.SiteEventsListOK {
		t.Helper()
		out, err := c.SiteEventsList(t.Context(), siteapi.SiteEventsListParams{
			WorkspaceId:    fixtures.AcmePublicID,
			Action:         siteapi.NewOptString(events.NameEmailOpened),
			ContactId:      siteapi.NewOptEntityId(siteapi.EntityId(fixtures.ContactAliceID)),
			IncludeMachine: siteapi.NewOptBool(includeMachine),
		})
		require.NoError(t, err)
		res, ok := out.(*siteapi.SiteEventsListOK)
		require.Truef(t, ok, "got %T", out)
		return res
	}

	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), scannerUA, personAddr)
	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), personUA, personAddr)
	persistEngagement(t, env)

	assert.EqualValues(t, 1, list(false).TotalItems, "the page and its total leave the machine open out")
	assert.EqualValues(t, 2, list(true).TotalItems)
}

func TestNeitherTheAddressNorTheUserAgentIsStored(t *testing.T) {
	env := testhelper.Setup(t)

	hit(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID), scannerUA, relayAddr)
	persistEngagement(t, env)

	for _, e := range env.OutboxEnvelopes(t, events.NameEmailOpened) {
		assert.NotContains(t, string(e.Data), relayAddr)
		assert.NotContains(t, string(e.Data), "Googlebot")
	}
	rows := env.DB.Event.Query().AllX(t.Context())
	for _, row := range rows {
		props, err := json.Marshal(row.Properties)
		require.NoError(t, err)
		assert.NotContains(t, string(props), relayAddr)
		assert.NotContains(t, string(props), "Googlebot")
	}
}

func TestRefreshReplacesTheRangesAndAFailedRefreshKeepsThem(t *testing.T) {
	env := testhelper.Setup(t)
	path := openPath(t, env, fixtures.OutboundMessageTrackingSampleID)
	machineHit := func(addr string) bool {
		t.Helper()
		before := len(env.OutboxEvents(t, events.NameEmailOpened))
		hit(t, env, path, personUA, addr)
		opened := env.OutboxEvents(t, events.NameEmailOpened)
		require.Len(t, opened, before+1)
		return opened[before].(*events.EmailEngagement).Machine
	}
	require.True(t, machineHit(relayAddr), "the fixture range is listed")
	require.False(t, machineHit(publishedAddr))

	env.PrivateRelay.Fail(errors.New("apple is down"))
	require.Error(t, env.Machine.Refresh(t.Context()))
	assert.True(t, machineHit(relayAddr), "a failed fetch keeps the old ranges")

	env.PrivateRelay.Publish("not a range\n\n")
	require.Error(t, env.Machine.Refresh(t.Context()))
	assert.True(t, machineHit(relayAddr), "a list with no range keeps the old ranges")

	env.PrivateRelay.Publish(publishedRowCSV)
	require.NoError(t, env.Machine.Refresh(t.Context()))
	assert.True(t, machineHit(publishedAddr), "the published range is listed")
	assert.False(t, machineHit(relayAddr), "the list is replaced as a whole")
}
