package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveHealthz starts a server on a loopback port answering /healthz with status and points
// PORT at it.
func serveHealthz(t *testing.T, status int) *int {
	t.Helper()
	hits := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
			*hits++
			w.WriteHeader(status)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	t.Setenv("PORT", port)
	return hits
}

func TestHealthcheckPassesWhenHealthzAnswersOK(t *testing.T) {
	hits := serveHealthz(t, http.StatusOK)

	require.NoError(t, execute(nil, &bytes.Buffer{}, "healthcheck"))

	assert.Equal(t, 1, *hits)
}

func TestHealthcheckFailsWhenHealthzAnswersAnotherStatus(t *testing.T) {
	serveHealthz(t, http.StatusServiceUnavailable)

	err := execute(nil, &bytes.Buffer{}, "healthcheck")

	require.ErrorContains(t, err, "503")
}

func TestHealthcheckFailsWhenNothingListens(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	require.NoError(t, ln.Close())
	t.Setenv("PORT", port)

	require.Error(t, execute(nil, &bytes.Buffer{}, "healthcheck"))
}

func TestHealthcheckRejectsArguments(t *testing.T) {
	require.Error(t, execute(nil, &bytes.Buffer{}, "healthcheck", "http://example.com"))
}
