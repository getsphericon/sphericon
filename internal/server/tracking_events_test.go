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

// Tracking names the Outbound message, and every open and click is an Event of its own (ADR 0053):
// the Broadcast recipient's rollup is set once and carries the unique figures.

const (
	linkA = "https://dest.test/a"
	linkB = "https://dest.test/b"
)

func openPath(t *testing.T, env *testhelper.TestEnv, messageID int64) string {
	t.Helper()
	token, err := env.Tracker.Token(messageID)
	require.NoError(t, err)
	return "/e/o/" + token
}

func linkClicks(t *testing.T, env *testhelper.TestEnv) map[string]int32 {
	t.Helper()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteBroadcastLinksList(t.Context(), siteapi.SiteBroadcastLinksListParams{
		WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID),
		BroadcastId: siteapi.EntityId(fixtures.BroadcastSentUntrackedID),
	})
	require.NoError(t, err)
	list := res
	out := map[string]int32{}
	for _, l := range list.Items {
		out[l.URL] = l.Clicks
	}
	return out
}

func TestEveryOpenIsAnEventAndTheRollupIsStampedOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	path := openPath(t, env, fixtures.OutboundMessageTrackingSampleID)

	for range 3 {
		assert.Equal(t, http.StatusOK, trackingGet(t, env, path).Code)
	}

	opened := env.QueuedEvents(t, events.NameEmailOpened)
	require.Len(t, opened, 3, "one email.opened Event per open")
	for _, e := range opened {
		got, ok := e.(*events.EmailEngagement)
		require.True(t, ok)
		assert.EqualValues(t, fixtures.OutboundMessageTrackingSampleID, got.OutboundMessageID)
		assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, got.BroadcastID)
	}
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	require.NotNil(t, env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageTrackingSampleID).OpenedAt)
	assert.Equal(t, 1, figures(t, env, fixtures.BroadcastSentUntrackedID).Opened, "unique opens count the recipient once")
}

// The first open and the first click are stamped at the Instance clock's time, not the wall's.
func TestOpenAndClickAreStampedAtTheInstanceClock(t *testing.T) {
	at := time.Now().Add(time.Hour)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	ctx := t.Context()
	openAt := openPath(t, env, fixtures.OutboundMessageTrackingSampleID)
	clickAt := clickPathTo(t, env, fixtures.OutboundMessageTrackingSampleID, linkA)

	opened := trackingGet(t, env, openAt)
	clicked := trackingGet(t, env, clickAt)

	require.Equal(t, http.StatusOK, opened.Code)
	require.Equal(t, http.StatusFound, clicked.Code)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	msg := env.DB.OutboundMessage.GetX(ctx, fixtures.OutboundMessageTrackingSampleID)
	require.NotNil(t, msg.OpenedAt)
	require.NotNil(t, msg.ClickedAt)
	assert.WithinDuration(t, at, *msg.OpenedAt, time.Microsecond, "opened at the Instance clock")
	assert.WithinDuration(t, at, *msg.ClickedAt, time.Microsecond, "clicked at the Instance clock")
}

func TestEveryClickIsAnEventWithItsURLAndTheReportListsTheLinks(t *testing.T) {
	env := testhelper.Setup(t)

	for _, dest := range []string{linkA, linkA, linkB} {
		link, err := env.Tracker.ClickURL(fixtures.OutboundMessageTrackingSampleID, dest)
		require.NoError(t, err)
		resp := trackingGet(t, env, "/e/c/"+link[len(env.Tracker.Origin()+"/e/c/"):])
		assert.Equal(t, http.StatusFound, resp.Code)
		assert.Equal(t, dest, resp.Header().Get("Location"))
	}

	clicked := env.QueuedEvents(t, events.NameEmailClicked)
	require.Len(t, clicked, 3, "one email.clicked Event per click")
	assert.Equal(t, linkA, clicked[0].(*events.EmailEngagement).URL)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	assert.Equal(t, 1, figures(t, env, fixtures.BroadcastSentUntrackedID).Clicked, "unique clicks count the recipient once")

	assert.Equal(t, map[string]int32{linkA: 2, linkB: 1}, linkClicks(t, env))
}

func TestUniqueFiguresSurviveTheExpiryOfTheEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	link, err := env.Tracker.ClickURL(fixtures.OutboundMessageTrackingSampleID, linkA)
	require.NoError(t, err)
	trackingGet(t, env, "/e/c/"+link[len(env.Tracker.Origin()+"/e/c/"):])
	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID))
	require.Equal(t, map[string]int32{linkA: 1}, linkClicks(t, env))

	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	n, err := events.PruneEvents(ctx, env.DB, time.Nanosecond, 100, time.Now())
	require.NoError(t, err)
	require.Positive(t, n, "the engagement Events are analytical and expire")

	assert.Empty(t, linkClicks(t, env), "the per-link counts cover the retention window")
	got := figures(t, env, fixtures.BroadcastSentUntrackedID)
	assert.Equal(t, 1, got.Opened)
	assert.Equal(t, 1, got.Clicked)
}
