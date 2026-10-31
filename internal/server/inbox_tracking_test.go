package server_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var (
	capturedPixel = regexp.MustCompile(`/e/o/[A-Za-z0-9_.-]+`)
	capturedClick = regexp.MustCompile(`/e/c/[A-Za-z0-9_.-]+`)
)

// A Broadcast sent through the Test inbox carries the real tracker's pixel and links: loading the
// pixel and following a link of the Captured message through the real tracking routes is a real
// open and a real click. Every open is an Event; the recipient's rollup counts it once (ADR 0053).
func TestOpeningAndFollowingALinkOfACapturedMessageIsTrackedLikeAnyOther(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	chosen, err := jane.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, chosen)
	edited, err := jane.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Body: siteapi.NewOptString(`<p>Read <a href="https://dest.test/a">this</a>.</p>`),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, edited)
	sent, err := jane.SiteBroadcastsSend(ctx, siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastResource{}, sent)
	env.DeliverQueuedBroadcasts(t)
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.Recipient(fixtures.ContactGlobexReaderEmail)).OnlyX(ctx)
	pixel := capturedPixel.FindString(kept.HTML)
	click := capturedClick.FindString(kept.HTML)
	require.NotEmpty(t, pixel, "the message carries the open pixel")
	require.NotEmpty(t, click, "and a tracked link")

	for range 2 {
		assert.Equal(t, http.StatusOK, trackingGet(t, env, pixel).Code)
	}
	followed := trackingGet(t, env, click)

	assert.Equal(t, http.StatusFound, followed.Code)
	assert.NotEmpty(t, followed.Header().Get("Location"))
	assert.Len(t, env.OutboxEvents(t, events.NameEmailOpened), 2, "one email.opened Event per hit")
	assert.Len(t, env.OutboxEvents(t, events.NameEmailClicked), 1)
	got, err := broadcasts.StatsOf(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastGlobexID)
	require.NoError(t, err)
	assert.Equal(t, 1, got[fixtures.BroadcastGlobexID].Opened, "the recipient's rollup counts the open once")
	assert.Equal(t, 1, got[fixtures.BroadcastGlobexID].Clicked)
}
