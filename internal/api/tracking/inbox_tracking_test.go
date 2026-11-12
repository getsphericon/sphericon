package tracking_test

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
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
	_, err := jane.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(ctx, t, int64(siteapi.EntityId(fixtures.IntegrationGlobexInboxID)))), IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)

	_, err = jane.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(siteapi.EntityId(fixtures.BroadcastGlobexID)))),
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString(`<p>Read <a href="https://dest.test/a">this</a>.</p>`)}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)

	_, err = jane.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(siteapi.EntityId(fixtures.BroadcastGlobexID))))}, siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)

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
	assert.Len(t, env.QueuedEvents(t, events.NameEmailOpened), 2, "one email.opened Event per hit")
	assert.Len(t, env.QueuedEvents(t, events.NameEmailClicked), 1)
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	got, err := broadcasts.StatsOf(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastGlobexID)
	require.NoError(t, err)
	assert.Equal(t, 1, got[fixtures.BroadcastGlobexID].Opened, "the recipient's rollup counts the open once")
	assert.Equal(t, 1, got[fixtures.BroadcastGlobexID].Clicked)
}
