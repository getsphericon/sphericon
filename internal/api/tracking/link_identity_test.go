package tracking_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Link identity token (ADR 0160) rides the click of a tracked link to the Workspace's own sites:
// the redirect adds it exactly when the click is recorded, the Workspace has turned it on, the
// destination is https on a listed host and the message went out less than 7 days ago.

// readerSite is the host of the Globex reader's link (readerLink).
const readerSite = "dest.test"

// linkIdentityParam is the query parameter the token travels in: the name the tracker reads
// (packages/analytics), written out so a rename on either side fails here.
const linkIdentityParam = "sph_lit"

// turnOnLinkIdentity turns Link identity tokens on for hosts, as the Workspace's Owner does.
func turnOnLinkIdentity(t *testing.T, env *testhelper.TestEnv, ownerEmail string, workspace siteapi.WorkspaceId, hosts ...string) {
	t.Helper()
	listed := make([]siteapi.LinkIdentityHost, 0, len(hosts))
	for _, h := range hosts {
		listed = append(listed, siteapi.LinkIdentityHost(h))
	}
	_, err := env.SiteActor(t, ownerEmail).SiteWorkspacesSetLinkIdentity(t.Context(), &siteapi.SiteSetLinkIdentityInput{
		Revision: siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(workspace))), LinkIdentityEnabled: true, LinkIdentityHosts: listed,
	}, siteapi.SiteWorkspacesSetLinkIdentityParams{WorkspaceId: workspace})
	require.NoError(t, err)
}

// readerClickTo is the click path of a link to dest in the message the Globex reader was sent.
func readerClickTo(t *testing.T, env *testhelper.TestEnv, dest string) string {
	t.Helper()
	id := env.DB.OutboundMessage.Query().Where(outboundmessage.Destination(fixtures.ContactGlobexReaderEmail), outboundmessage.BroadcastID(fixtures.BroadcastGlobexID)).OnlyIDX(t.Context())
	return clickPathTo(t, env, id, dest)
}

// splitToken parses a redirect's Location into the destination without the token and the token.
func splitToken(t *testing.T, location string) (dest, token string) {
	t.Helper()
	u, err := url.Parse(location)
	require.NoError(t, err)
	token = u.Query().Get(linkIdentityParam)
	u.RawQuery = strings.TrimPrefix(strings.TrimSuffix(strings.Replace(u.RawQuery, linkIdentityParam+"="+token, "", 1), "&"), "&")
	return u.String(), token
}

// The click of a tracked link to a listed site carries the recipient's token, which names their
// Contact in the Workspace.
func TestAClickToAListedSiteCarriesTheRecipientsLinkIdentityToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	html := sendToReader(t, env)

	click := capturedClick.FindString(html)
	followed := trackingGet(t, env, click)

	require.Equal(t, http.StatusFound, followed.Code)
	dest, token := splitToken(t, followed.Header().Get("Location"))
	assert.Equal(t, readerLink, dest)
	got, err := env.Tracker.DecodeLinkIdentity(token)
	require.NoError(t, err)
	assert.Equal(t, messagetoken.LinkIdentity{WorkspaceID: fixtures.GlobexID, ContactID: fixtures.ContactGlobexReaderID}, got)
}

// The token joins the destination's own query and leaves its fragment where it is.
func TestTheTokenKeepsTheDestinationsQueryAndFragment(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	sendToReader(t, env)

	click := readerClickTo(t, env, "https://dest.test/a?utm_source=mail&q=a%20b#top")
	followed := trackingGet(t, env, click)

	location := followed.Header().Get("Location")
	assert.True(t, strings.HasPrefix(location, "https://dest.test/a?utm_source=mail&q=a%20b&"+linkIdentityParam+"="), location)
	assert.True(t, strings.HasSuffix(location, "#top"), location)
}

// A machine's click gets it too: the tracker keeps a scanner out on the site.
func TestAMachinesClickCarriesTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	sendToReader(t, env)

	click := readerClickTo(t, env, readerLink)
	followed := hit(t, env, click, scannerUA, personAddr)

	_, token := splitToken(t, followed.Header().Get("Location"))
	assert.NotEmpty(t, token)
}

// A Short tracked link of an SMS or a Telegram message carries it as an email's link does.
func TestAShortTrackedLinkCarriesTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "example.com")

	short := env.DB.TrackedLink.GetX(t.Context(), fixtures.TrackedLinkAcmeID)
	followed := trackingGet(t, env, "/c/"+short.Token)

	require.Equal(t, http.StatusFound, followed.Code)
	dest, token := splitToken(t, followed.Header().Get("Location"))
	assert.Equal(t, fixtures.TrackedLinkAcmeURL, dest)
	got, err := env.Tracker.DecodeLinkIdentity(token)
	require.NoError(t, err)
	assert.Equal(t, messagetoken.LinkIdentity{WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactLiamID}, got)
}

// Switched off, no link carries it.
func TestWithLinkIdentityOffNoClickCarriesTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	html := sendToReader(t, env)

	click := capturedClick.FindString(html)
	followed := trackingGet(t, env, click)

	assert.Equal(t, readerLink, followed.Header().Get("Location"))
}

// A link to a site off the list, or to a listed site over plain http, redirects without it.
func TestOnlyAnHTTPSLinkToAListedSiteCarriesTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	sendToReader(t, env)

	for _, dest := range []string{"https://other.test/a", "https://shop.dest.test/a", "http://dest.test/a"} {
		t.Run(dest, func(t *testing.T) {
			click := readerClickTo(t, env, dest)
			followed := trackingGet(t, env, click)

			assert.Equal(t, dest, followed.Header().Get("Location"))
		})
	}
}

// A message sent 8 days ago carries no token; one sent 6 days ago still does.
func TestAMessageSentMoreThanSevenDaysAgoCarriesNoToken(t *testing.T) {
	clock := testhelper.NewClock(time.Now())
	env := testhelper.Setup(t, testhelper.WithClock(clock.Now))
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	sendToReader(t, env)

	click := readerClickTo(t, env, readerLink)

	clock.Advance(6 * 24 * time.Hour)
	sixDays := trackingGet(t, env, click)
	clock.Advance(2 * 24 * time.Hour)
	stale := trackingGet(t, env, click)

	_, fresh := splitToken(t, sixDays.Header().Get("Location"))

	assert.NotEmpty(t, fresh)
	assert.Equal(t, readerLink, stale.Header().Get("Location"))
}

// A message that went out untracked records no click, and its link carries no token.
func TestAnUntrackedMessagesLinkCarriesNoToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	sendToReader(t, env)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionTrack)
	click := readerClickTo(t, env, readerLink)

	followed := trackingGet(t, env, click)

	assert.Equal(t, readerLink, followed.Header().Get("Location"))
}

// A message whose tracking was withdrawn carries none, even once the default tracks again.
func TestAMessageWhoseTrackingWasWithdrawnCarriesNoToken(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	html := sendToReader(t, env)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionDontTrack)
	withdraw(t, env)
	setGlobexDefault(t, env, siteapi.EngagementTrackingDecisionTrack)

	click := capturedClick.FindString(html)
	followed := trackingGet(t, env, click)

	assert.Equal(t, readerLink, followed.Header().Get("Location"))
}

// A Destination that refused tracking after the send gets none.
func TestARefusalAfterTheSendTakesTheTokenOffTheLink(t *testing.T) {
	env := testhelper.Setup(t)
	turnOnLinkIdentity(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, readerSite)
	html := sendToReader(t, env)
	globexTracking(t, env, fixtures.ContactGlobexReaderEmail, externalapi.EngagementTrackingStateDenied)

	click := capturedClick.FindString(html)
	followed := trackingGet(t, env, click)

	assert.Equal(t, readerLink, followed.Header().Get("Location"))
}
