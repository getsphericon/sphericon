package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const edgeHeader = "X-Edge-Host"

func trackHost(domain string) string { return "track." + domain }

func hostReq(env *testhelper.TestEnv, method, host, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
	if host != "" {
		req.Host = host
	}
	w := httptest.NewRecorder()
	env.Server.ServeHTTP(w, req)
	return w
}

// A verified Tracking host serves the link paths and nothing else (ADR 0038).
func TestTrackingHostServesOnlyLinkPaths(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	tr := tracking.New(cfg.JWTSecret, cfg.AppURL)
	host := trackHost(fixtures.SendingDomainTrackingServingDomain)

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
	b, err := env.DB.Broadcast.Create().SetWorkspaceID(fixtures.AcmeID).SetName("T").SetSubject("S").SetStatus(broadcast.StatusSent).SetSentAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	rec, err := env.DB.BroadcastRecipient.Create().SetWorkspaceID(fixtures.AcmeID).SetBroadcastID(b.ID).SetContactID(fixtures.ContactAliceID).Save(ctx)
	require.NoError(t, err)
	rtoken, err := tr.Token(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, "/e/o/"+rtoken).Code)
	assert.Equal(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, "/e/c/"+rtoken+"?u=https://example.com").Code)
	// On the Instance's own host the same recipient still works.
	assert.Equal(t, http.StatusFound, hostReq(env, http.MethodGet, "", "/e/c/"+rtoken+"?u=https://example.com").Code)

	// Everything else is a 404: the SPA, the API prefixes, sign-in, probes.
	for _, p := range []string{"/", "/login", "/site/workspaces", "/api/contacts", "/mcp", "/collect/v1/batch", "/healthz", "/t.js", "/staff/me"} {
		assert.Equalf(t, http.StatusNotFound, hostReq(env, http.MethodGet, host, p).Code, "path %s", p)
	}
}

// The probe is served on a dns_verified host as well, with a value tied to the host.
func TestTrackingHostServesTheProbeValue(t *testing.T) {
	env := testhelper.Setup(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)

	for _, domain := range []string{fixtures.SendingDomainTrackingServingDomain, fixtures.SendingDomainTrackingDNSVerifiedDomain} {
		host := trackHost(domain)
		resp := hostReq(env, http.MethodGet, host+":443", trackingdomain.ProbePath)
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, trackingdomain.ProbeValue(cfg.JWTSecret, host), resp.Body.String())
	}
	assert.NotEqual(t,
		trackingdomain.ProbeValue(cfg.JWTSecret, trackHost(fixtures.SendingDomainTrackingServingDomain)),
		trackingdomain.ProbeValue(cfg.JWTSecret, trackHost(fixtures.SendingDomainTrackingDNSVerifiedDomain)))

	// A pending host is not a Tracking host: the request falls through as any other.
	pending := trackHost(fixtures.SendingDomainTrackingPendingDomain)
	resp := hostReq(env, http.MethodGet, pending, trackingdomain.ProbePath)
	assert.NotEqual(t, trackingdomain.ProbeValue(cfg.JWTSecret, pending), resp.Body.String())
}

// viaPeer serves GET /healthz from a peer address, with the host header set when given.
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
	host := trackHost(fixtures.SendingDomainTrackingServingDomain)

	// From the edge the header selects the tracking host: /healthz is not a link path.
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "127.0.0.1:4444", host))
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "[::1]:4444", host))
	// An unverified host named by the edge is a 404, never the SPA.
	assert.Equal(t, http.StatusNotFound, viaPeer(env, "127.0.0.1:4444", "track.nobody.test"))
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
	host := trackHost(fixtures.SendingDomainTrackingServingDomain)

	assert.Equal(t, http.StatusNotFound, viaPeer(env, "10.1.2.3:4444", host))
	assert.Equal(t, http.StatusOK, viaPeer(env, "10.2.2.3:4444", host))
	assert.Equal(t, http.StatusOK, viaPeer(env, "127.0.0.1:4444", host))
}
