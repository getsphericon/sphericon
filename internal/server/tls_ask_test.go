package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/fixtures"
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
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, fixtures.TrackingDomainDNSVerifiedHost))
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, fixtures.TrackingDomainServingHost))
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, fixtures.TrackingDomainRetiredHost), "a retired host is still served")
			assert.Equal(t, http.StatusOK, ask(t, env.Internal, strings.ToUpper(fixtures.TrackingDomainServingHost)+"."))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, fixtures.TrackingDomainReplacementHost))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, "t.unknown.example"))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, "umbrella.test"), "the bare domain is not the tracking host")
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, ""))
			assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, "evil/"+fixtures.TrackingDomainServingHost))
		})
	}
}

// The public mux's catch-all answers unknown paths alike, so the ask must answer
// exactly like one, even for a host that would be answered 200 on the internal listener.
func TestTLSAskIsNotOnThePublicMux(t *testing.T) {
	env := testhelper.Setup(t)
	host := fixtures.TrackingDomainServingHost

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
	host := fixtures.TrackingDomainServingHost
	require.Equal(t, http.StatusOK, ask(t, env.Internal, host))

	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).SetDiscardedAt(time.Now()).ExecX(t.Context())
	assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host))

	env.FeedDB.Workspace.UpdateOneID(fixtures.UmbrellaID).ClearDiscardedAt().ExecX(t.Context())
	assert.Equal(t, http.StatusOK, ask(t, env.Internal, host))
}

// A Tracking domain that goes is refused at once, so the edge stops issuing certificates for it.
func TestTLSAskRefusesAHostThatWent(t *testing.T) {
	env := testhelper.Setup(t)
	host := fixtures.TrackingDomainRetiredHost
	require.Equal(t, http.StatusOK, ask(t, env.Internal, host))

	require.NoError(t, env.FeedDB.Scoped(fixtures.UmbrellaID).TrackingDomain().
		DeleteOneID(fixtures.TrackingDomainRetiredID).Exec(t.Context()))
	assert.Equal(t, http.StatusNotFound, ask(t, env.Internal, host))
}
