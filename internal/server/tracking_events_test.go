package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Tracking names the Outbound message, and every open and click is an Event of its own (ADR 0053):
// the Broadcast recipient's rollup is set once and carries the unique figures.

const (
	linkA = "https://dest.test/a"
	linkB = "https://dest.test/b"
)

// persistEngagement writes the Event rows the persist subscriber would for the opens and clicks
// published so far, as the router does not run under the test transaction.
func persistEngagement(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	envelopes := env.OutboxEnvelopes(t, events.NameEmailOpened, events.NameEmailClicked)
	for i := range envelopes {
		require.NoError(t, events.Persist(t.Context(), env.DB, envelopes[i]))
	}
}

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
	list, ok := res.(*siteapi.SiteBroadcastLinkClicksList)
	require.True(t, ok, "got %T", res)
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

	opened := env.OutboxEvents(t, events.NameEmailOpened)
	require.Len(t, opened, 3, "one email.opened Event per open")
	for _, e := range opened {
		got, ok := e.(*events.EmailEngagement)
		require.True(t, ok)
		assert.EqualValues(t, fixtures.OutboundMessageTrackingSampleID, got.OutboundMessageID)
		assert.EqualValues(t, fixtures.BroadcastSentUntrackedID, got.BroadcastID)
	}
	rec := env.DB.BroadcastRecipient.GetX(ctx, fixtures.BroadcastRecipientUntrackedID)
	require.NotNil(t, rec.OpenedAt)
	assert.Equal(t, 1, figures(t, env, fixtures.BroadcastSentUntrackedID).Opened, "unique opens count the recipient once")
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

	clicked := env.OutboxEvents(t, events.NameEmailClicked)
	require.Len(t, clicked, 3, "one email.clicked Event per click")
	assert.Equal(t, linkA, clicked[0].(*events.EmailEngagement).URL)
	assert.Equal(t, 1, figures(t, env, fixtures.BroadcastSentUntrackedID).Clicked, "unique clicks count the recipient once")

	persistEngagement(t, env)
	assert.Equal(t, map[string]int32{linkA: 2, linkB: 1}, linkClicks(t, env))
}

func TestUniqueFiguresSurviveTheExpiryOfTheEvents(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	link, err := env.Tracker.ClickURL(fixtures.OutboundMessageTrackingSampleID, linkA)
	require.NoError(t, err)
	trackingGet(t, env, "/e/c/"+link[len(env.Tracker.Origin()+"/e/c/"):])
	trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageTrackingSampleID))
	persistEngagement(t, env)
	require.Equal(t, map[string]int32{linkA: 1}, linkClicks(t, env))

	n, err := events.PruneEvents(ctx, env.DB, time.Nanosecond, 100)
	require.NoError(t, err)
	require.Positive(t, n, "the engagement Events are analytical and expire")

	assert.Empty(t, linkClicks(t, env), "the per-link counts cover the retention window")
	got := figures(t, env, fixtures.BroadcastSentUntrackedID)
	assert.Equal(t, 1, got.Opened)
	assert.Equal(t, 1, got.Clicked)
}
