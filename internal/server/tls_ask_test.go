package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
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
