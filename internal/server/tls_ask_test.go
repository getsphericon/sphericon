package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ask(t *testing.T, h http.Handler, domain string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/tls/ask?domain="+url.QueryEscape(domain), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code
}

func TestTLSAskAnswersForVerifiedTrackingHostsOnly(t *testing.T) {
	for _, license := range []struct {
		name string
		opts []testhelper.Option
	}{{"licensed", nil}, {"unlicensed", []testhelper.Option{testhelper.WithoutLicense()}}} {
		t.Run(license.name, func(t *testing.T) {
			env := testhelper.Setup(t, license.opts...)
			host := sendingdomains.TrackingHost

			assert.Equal(t, http.StatusOK, ask(t, env.Internal, host(fixtures.SendingDomainTrackingDNSVerifiedDomain)))
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, host(fixtures.SendingDomainTrackingServingDomain)))
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, strings.ToUpper(host(fixtures.SendingDomainTrackingServingDomain))+"."))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host(fixtures.SendingDomainTrackingPendingDomain)))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host("unknown.example")))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, fixtures.SendingDomainTrackingServingDomain), "the bare domain is not the tracking host")
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, ""))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, "track."))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, "track.evil/"+fixtures.SendingDomainTrackingServingDomain))
		})
	}
}

// The public mux's catch-all answers unknown paths alike, so the ask must answer
// exactly like one, even for a host that would be answered 200 on the internal listener.
func TestTLSAskIsNotOnThePublicMux(t *testing.T) {
	env := testhelper.Setup(t)
	host := sendingdomains.TrackingHost(fixtures.SendingDomainTrackingServingDomain)

	serve := func(target string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		env.Server.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
		return w
	}
	asked, unknown := serve("/tls/ask?domain="+url.QueryEscape(host)), serve("/definitely-unknown")
	assert.Equal(t, unknown.Code, asked.Code)
	assert.Equal(t, unknown.Body.String(), asked.Body.String())
}

// A pending-deletion Workspace's tracking hosts are refused, so the edge stops issuing
// certificates for them (ADR 0043); restoring brings them back.
func TestTLSAskRefusesTheHostsOfADeletedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	host := sendingdomains.TrackingHost(fixtures.SendingDomainTrackingServingDomain)
	require.Equal(t, http.StatusOK, ask(t, env.Internal, host))

	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetDiscardedAt(time.Now()).ExecX(t.Context())
	assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host))

	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).ClearDiscardedAt().ExecX(t.Context())
	assert.Equal(t, http.StatusOK, ask(t, env.Internal, host))
}

// An archived Sending domain serves no Tracking host (ADR 0049): the ask refuses the host of a
// fixture row that was serving when it was archived, and archiving a serving host makes the edge
// refuse it; restoring the row brings it back.
func TestTLSAskRefusesTheHostOfAnArchivedSendingDomain(t *testing.T) {
	env := testhelper.Setup(t)
	assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, sendingdomains.TrackingHost(fixtures.SendingDomainArchivedDomain)))

	host := sendingdomains.TrackingHost(fixtures.SendingDomainTrackingServingDomain)
	require.Equal(t, http.StatusOK, ask(t, env.Internal, host))
	umbrella := env.FeedDB.Scoped(fixtures.UmbrellaID)

	require.NoError(t, umbrella.SendingDomain().Discard(t.Context(), fixtures.SendingDomainTrackingServingID))
	assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host))

	require.NoError(t, umbrella.SendingDomain().Undiscard(t.Context(), fixtures.SendingDomainTrackingServingID))
	assert.Equal(t, http.StatusOK, ask(t, env.Internal, host))
}
