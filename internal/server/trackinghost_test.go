package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"entgo.io/ent"
	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const edgeHeader = "X-Edge-Host"

// The tracker's own host and a name that cannot be a Tracking domain cost the gate no query: the
// tracker's traffic is the heaviest the Instance takes.
func TestTrackingHostGateLooksUpOnlyNamesThatCanBeATrackingDomain(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackerURL = "https://t.sphericon.example" }))
	lookups := 0
	env.DB.TrackingDomain.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
			lookups++
			return next.Query(ctx, q)
		})
	}))

	for _, host := range []string{"t.sphericon.example", "203.0.113.9", "localhost", "umbrella.test"} {
		hostReq(env, http.MethodGet, host, "/t.js")
	}
	assert.Zero(t, lookups)

	hostReq(env, http.MethodGet, fixtures.TrackingDomainServingHost, "/t.js")
	assert.Equal(t, 1, lookups, "a candidate host is looked up")
}

func hostReq(env *testhelper.TestEnv, method, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	if host != "" {
		req.Host = host
	}
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)
	return w
}

// A verified Tracking host serves the link paths, scoped to its own Workspace, and no other
// surface of the Instance (ADR 0038).
func TestTrackingHostScopesLinkPathsToItsWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	tr := tracking.New(cfg.JWTSecret, cfg.AppURL)
	host := fixtures.TrackingDomainServingHost

	unsubFor := func(ws int64) string {
		full, err := tr.UnsubscribeURL(tracking.UnsubTarget{
			Source: eligibility.SourceBroadcasts, Destination: "pat@umbrella.test", WorkspaceID: ws,
		})
		require.NoError(t, err)
		_, token, _ := strings.Cut(full, "/e/u/")
		return "/e/u/" + token
	}
	own := unsubFor(fixtures.UmbrellaID)

	// The page link redirects to the Instance's own host with the same token.
	resp := hostReq(env, http.MethodGet, host, own)
	assert.Equal(t, http.StatusSeeOther, resp.Code)
	assert.Equal(t, cfg.AppURL+"/unsubscribe?token="+strings.TrimPrefix(own, "/e/u/"), strings.SplitN(resp.Header().Get("Location"), "&", 2)[0])

	// The one-click POST opts the destination out in the host's Workspace.
	resp = hostReq(env, http.MethodPost, host, own)
	assert.Equal(t, http.StatusNoContent, resp.Code)
	ok, err := env.DB.Unsubscribe.Query().Where(
		unsubscribe.WorkspaceID(fixtures.UmbrellaID), unsubscribe.DestinationEQ("pat@umbrella.test"),
		unsubscribe.SendingSourceEQ(eligibility.SourceBroadcasts),
	).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, ok)

	// Another Workspace's token is a 404 here, GET and POST.
	foreign := unsubFor(fixtures.AcmeID)
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, foreign).Code)
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodPost, host, foreign).Code)

	// Confirm links redirect to the Instance's host too.
	confirmURL, err := tr.ConfirmURL(tracking.ConfirmTarget{Destination: "pat@umbrella.test", WorkspaceID: fixtures.UmbrellaID, ContactID: 1})
	require.NoError(t, err)
	_, ctoken, _ := strings.Cut(confirmURL, "/e/confirm/")
	resp = hostReq(env, http.MethodGet, host, "/e/confirm/"+ctoken)
	assert.Equal(t, http.StatusSeeOther, resp.Code)
	assert.True(t, strings.HasPrefix(resp.Header().Get("Location"), cfg.AppURL+"/confirm?token="))

	// Open and click of a recipient of another Workspace are a 404.
	rtoken, err := tr.Token(fixtures.OutboundMessageTrackingSampleID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, "/e/o/"+rtoken).Code)
	clickLink, err := tr.ClickURL(fixtures.OutboundMessageTrackingSampleID, "https://example.com")
	require.NoError(t, err)
	_, ctok, _ := strings.Cut(clickLink, "/e/c/")
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, "/e/c/"+ctok).Code)
	// On the Instance's own host the same recipient still works.
	assert.Equal(t, http.StatusFound, hostReq(env, http.MethodGet, "", "/e/c/"+ctok).Code)

	// Everything else is a 404: the SPA, the API prefixes, sign-in, probes.
	for _, p := range []string{"/", "/login", "/site/workspaces", "/api/contacts", "/mcp", "/healthz", "/staff/me"} {
		assert.Equalf(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, p).Code, "path %s", p)
	}
}

// A verified Tracking host also serves the first-party tracker (ADR 0059): the script, and the
// collect API for its own Workspace's key only.
func TestTrackingHostServesTheFirstPartyTracker(t *testing.T) {
	env := testhelper.Setup(t)

	collect := func(host, key string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/collect/events", strings.NewReader(`{"events":[{"visitorId":"v1","action":"test.event"}]}`))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Collect-Key", key)
		w := httptest.NewRecorder()
		env.Server.ServeHTTP(w, req)
		return w.Code
	}

	// A host serves the tracker from dns_verified on, like the probe (ADR 0059), and a retired
	// host keeps serving the snippets installed on it (ADR 0071).
	for host, key := range map[string]string{
		fixtures.TrackingDomainServingHost:     fixtures.UmbrellaCollectKey,
		fixtures.TrackingDomainRetiredHost:     fixtures.UmbrellaCollectKey,
		fixtures.TrackingDomainDNSVerifiedHost: fixtures.InitechCollectKey,
	} {
		script := hostReq(env, http.MethodGet, host, "/t.js")
		assert.Equalf(t, http.StatusOK, script.Code, "script on %s", host)
		assert.Contains(t, script.Header().Get("Content-Type"), "javascript")
		assert.Equalf(t, http.StatusNoContent, collect(host, key), "the host's own key on %s", host)
		assert.Equalf(t, http.StatusUnauthorized, collect(host, fixtures.AcmeCollectKey), "another Workspace's key on %s is refused like a wrong one", host)
	}
}

// The probe is served on a dns_verified host as well, with a value tied to the host.
func TestTrackingHostServesTheProbeValue(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)

	for _, host := range []string{fixtures.TrackingDomainServingHost, fixtures.TrackingDomainDNSVerifiedHost, fixtures.TrackingDomainRetiredHost} {
		resp := hostReq(env, http.MethodGet, host+":443", trackingdomains.ProbePath)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, trackingdomains.ProbeValue(cfg.JWTSecret, host), resp.Body.String())
	}
	assert.NotEqual(t,
		trackingdomains.ProbeValue(cfg.JWTSecret, fixtures.TrackingDomainServingHost),
		trackingdomains.ProbeValue(cfg.JWTSecret, fixtures.TrackingDomainDNSVerifiedHost))

	// A pending host is not a Tracking host: the request falls through as any other.
	pending := fixtures.TrackingDomainReplacementHost
	resp := hostReq(env, http.MethodGet, pending, trackingdomains.ProbePath)
	assert.NotEqual(t, trackingdomains.ProbeValue(cfg.JWTSecret, pending), resp.Body.String())
}

// viaPeer serves GET /healthz from a peer address, with the host header set when given.
// Through the edge's header the tracker paths are scoped to the host's Workspace too (ADR 0059).
func TestTrackingHostFromTheEdgeServesTheTrackerForItsWorkspaceOnly(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackingHostHeader = edgeHeader }))
	host := fixtures.TrackingDomainServingHost

	collect := func(key string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/collect/events", strings.NewReader(`{"events":[{"visitorId":"v1","action":"test.event"}]}`))
		req.RemoteAddr = "127.0.0.1:4444"
		req.Header.Set(edgeHeader, host)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Collect-Key", key)
		w := httptest.NewRecorder()
		env.Server.ServeHTTP(w, req)
		return w.Code
	}
	assert.Equal(t, http.StatusNoContent, collect(fixtures.UmbrellaCollectKey))
	assert.Equal(t, http.StatusUnauthorized, collect(fixtures.AcmeCollectKey))
}

func viaPeer(env *testhelper.TestEnv, peer, header string) int {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	req.RemoteAddr = peer
	if header != "" {
		req.Header.Set(edgeHeader, header)
	}
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)
	return w.Code
}

// By default only loopback is the edge: a private-range peer of a shared network is not.
func TestTrackingHostHeaderIsTrustedOnlyFromLoopbackByDefault(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackingHostHeader = edgeHeader }))
	host := fixtures.TrackingDomainServingHost

	// From the edge the header selects the tracking host: /healthz is not a link path.
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "127.0.0.1:4444", host))
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "[::1]:4444", host))
	// An unverified host named by the edge is a 404, never the SPA.
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "127.0.0.1:4444", "t.nobody.test"))
	// From any other peer, private ranges included, the header is ignored.
	assert.Equal(t, http.StatusOK, viaPeer(env, "10.1.2.3:4444", host))
	assert.Equal(t, http.StatusOK, viaPeer(env, "203.0.113.9:4444", host))
	// No header: the connection's Host rules.
	assert.Equal(t, http.StatusOK, viaPeer(env, "127.0.0.1:4444", ""))
}

// An operator lists their proxy's range explicitly, and only that range is trusted.
func TestTrackingHostHeaderIsTrustedFromTheConfiguredRanges(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) {
		c.TrackingHostHeader = edgeHeader
		c.TrackingEdgeCIDRs = []netip.Prefix{netip.MustParsePrefix("10.1.0.0/16")}
	}))
	host := fixtures.TrackingDomainServingHost

	assert.Equal(t, http.StatusNotFound, viaPeer(env, "10.1.2.3:4444", host))
	assert.Equal(t, http.StatusOK, viaPeer(env, "10.2.2.3:4444", host))
	assert.Equal(t, http.StatusOK, viaPeer(env, "127.0.0.1:4444", host))
}
