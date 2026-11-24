package tracking_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/capturedmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Engagement tracking consent decides at render and at recording (ADR 0120): a Broadcast email to a
// Destination that is not tracked carries no pixel and its links as written, and an open or a
// click of a message that is not tracked records nothing, while the pixel is served and the link
// redirects.

// readerLink is the link of the Globex Broadcast these tests send to Globex's reader.
const readerLink = "https://dest.test/a"

// globexTracking records an Engagement tracking observation of a Globex address through /api, as
// the customer does.
func globexTracking(t *testing.T, env *testhelper.TestEnv, address string, state externalapi.EngagementTrackingState) {
	t.Helper()
	c := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.GlobexID, apitokens.ScopeContactsWrite))
	_, err := c.EngagementTrackingCreate(t.Context(), &externalapi.RecordEngagementTrackingInput{
		Channel: externalapi.EngagementTrackingChannelEmail, Destination: address, State: state,
	})
	require.NoError(t, err)
}

// setGlobexDefault sets Globex's Engagement tracking default through /site, as its Owner does.
func setGlobexDefault(t *testing.T, env *testhelper.TestEnv, d siteapi.EngagementTrackingDecision) {
	t.Helper()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := jane.SiteWorkspacesSetEngagementTrackingDefault(t.Context(), &siteapi.SiteSetEngagementTrackingDefaultInput{
		Revision: siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.GlobexPublicID))), EngagementTrackingDefault: d,
	}, siteapi.SiteWorkspacesSetEngagementTrackingDefaultParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
}

// sendToReader sends the Globex Broadcast, with one link, through Globex's Test inbox and returns
// the HTML Globex's reader received.
func sendToReader(t *testing.T, env *testhelper.TestEnv) string {
	t.Helper()
	ctx := t.Context()
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := jane.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Revision: siteapi.Revision(env.IntegrationRevision(ctx, t, int64(siteapi.EntityId(fixtures.IntegrationGlobexInboxID)))), IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.IntegrationGlobexInboxID)})
	require.NoError(t, err)
	_, err = jane.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(siteapi.EntityId(fixtures.BroadcastGlobexID)))),
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString(`<p>Read <a href="` + readerLink + `">this</a>.</p>`)}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID)})
	require.NoError(t, err)
	_, err = jane.SiteBroadcastsSend(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.BroadcastRevision(ctx, t, int64(siteapi.EntityId(fixtures.BroadcastGlobexID))))}, siteapi.SiteBroadcastsSendParams{
		WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.BroadcastGlobexID),
	})
	require.NoError(t, err)
	env.DeliverQueuedBroadcasts(t)
	kept := env.DB.CapturedMessage.Query().Where(capturedmessage.Recipient(fixtures.ContactGlobexReaderEmail)).OnlyX(ctx)
	return kept.HTML
}

// globexFigures is the Globex Broadcast's opened and clicked figures, once the stamps' projection
// ran.
func globexFigures(t *testing.T, env *testhelper.TestEnv) (opened, clicked int) {
	t.Helper()
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
	got, err := testhelper.Module[*broadcasts.Module](env).StatsOf(t.Context(), env.DB.Scoped(fixtures.GlobexID), fixtures.BroadcastGlobexID)
	require.NoError(t, err)
	return got[fixtures.BroadcastGlobexID].Opened, got[fixtures.BroadcastGlobexID].Clicked
}

// A Destination that refused tracking gets the next Broadcast email with no pixel and its link as
// written, and keeps its unsubscribe footer.
func TestABroadcastEmailToADestinationThatRefusedTrackingIsNotTracked(t *testing.T) {
	env := testhelper.Setup(t)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)

	html := sendToReader(t, env)

	assert.Empty(t, capturedPixel.FindString(html), "no open pixel")
	assert.Empty(t, capturedClick.FindString(html), "no rewritten link")
	assert.Contains(t, html, `href="`+readerLink+`"`, "the link as written")
	assert.Contains(t, html, "/e/u/", "the unsubscribe footer stays")
}

// A refusal recorded after a message went out reaches it: opening it and following its link record
// nothing, while the pixel is served and the link redirects.
func TestARefusalRecordedAfterASendRecordsNothingOfThatMessage(t *testing.T) {
	env := testhelper.Setup(t)
	html := sendToReader(t, env)
	pixel := capturedPixel.FindString(html)
	click := capturedClick.FindString(html)
	require.NotEmpty(t, pixel, "the message went out tracked")
	require.NotEmpty(t, click)

	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)
	opened := trackingGet(t, env, pixel)
	followed := trackingGet(t, env, click)

	assert.Equal(t, http.StatusOK, opened.Code, "the pixel is served")
	assert.Equal(t, http.StatusFound, followed.Code, "the link redirects")
	assert.Equal(t, readerLink, followed.Header().Get("Location"))
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened), "no open is recorded")
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailClicked), "no click is recorded")
	openedCount, clickedCount := globexFigures(t, env)
	assert.Zero(t, openedCount)
	assert.Zero(t, clickedCount)
}

// Under a default of dont_track, a Destination with no consent gets an untracked email.
func TestUnderDontTrackADestinationWithNoConsentIsNotTracked(t *testing.T) {
	env := testhelper.Setup(t)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)

	html := sendToReader(t, env)

	assert.Empty(t, capturedPixel.FindString(html))
	assert.Contains(t, html, `href="`+readerLink+`"`)
}

// Under a default of dont_track, a Destination that granted its consent is tracked: its open and
// its click are recorded and counted.
func TestUnderDontTrackADestinationThatGrantedIsTracked(t *testing.T) {
	env := testhelper.Setup(t)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateGranted)

	html := sendToReader(t, env)
	pixel := capturedPixel.FindString(html)
	click := capturedClick.FindString(html)
	require.NotEmpty(t, pixel)
	require.NotEmpty(t, click)
	trackingGet(t, env, pixel)
	trackingGet(t, env, click)

	opened, clicked := globexFigures(t, env)
	assert.Equal(t, 1, opened)
	assert.Equal(t, 1, clicked)
}

// The default is read when a hit arrives: a message that went out tracked records nothing once
// the default turned to dont_track, before any withdrawal reached it.
func TestAMessageSentUnderTrackRecordsNothingOnceTheDefaultIsDontTrack(t *testing.T) {
	env := testhelper.Setup(t)
	pixel := capturedPixel.FindString(sendToReader(t, env))
	require.NotEmpty(t, pixel)

	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	opened := trackingGet(t, env, pixel)

	assert.Equal(t, http.StatusOK, opened.Code)
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened))
}

// The open's Event names the address the message went to, not the one its Contact has since.
func TestAnOpenNamesTheAddressTheMessageWentToAfterItsContactChangedEmail(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	pixel := capturedPixel.FindString(sendToReader(t, env))
	require.NotEmpty(t, pixel)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	_, err := jane.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		Revision: siteapi.Revision(env.ContactRevision(ctx, t, fixtures.ContactGlobexReaderID)),
		Email:    siteapi.NewOptNilEmailAddress(siteapi.EmailAddress("moved@globex.test")),
	}, siteapi.SiteContactsUpdateParams{WorkspaceId: fixtures.GlobexPublicID, ID: siteapi.EntityId(fixtures.ContactGlobexReaderID)})
	require.NoError(t, err)

	trackingGet(t, env, pixel)

	opens := testhelper.Queued[*events.EmailEngagement](t, env, systemevent.EmailOpened)
	require.Len(t, opens, 1)
	assert.Equal(t, fixtures.ContactGlobexReaderEmail, opens[0].Email, "the address the message went to")
	assert.EqualValues(t, fixtures.ContactGlobexReaderID, opens[0].ContactID)
}

// A message whose tracking was withdrawn records nothing, whatever its Destination's consent.
func TestAMessageWhoseTrackingWasWithdrawnRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)

	opened := trackingGet(t, env, openPath(t, env, fixtures.OutboundMessageTrackingWithdrawnID))
	followed := trackingGet(t, env, clickPath(t, env, fixtures.OutboundMessageTrackingWithdrawnID))

	assert.Equal(t, http.StatusOK, opened.Code)
	assert.Equal(t, http.StatusFound, followed.Code)
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailOpened))
	assert.Empty(t, env.QueuedEvents(t, systemevent.EmailClicked))
}
