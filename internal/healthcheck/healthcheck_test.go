package healthcheck_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/healthcheck"
)

// serveHealthz starts a server on a loopback port answering /healthz with status and returns
// the port.
func serveHealthz(t *testing.T, status int) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	return port
}

func TestProbePassesWhenHealthzAnswersOK(t *testing.T) {
	port := serveHealthz(t, http.StatusOK)

	err := healthcheck.Probe(t.Context(), http.DefaultClient, port)

	require.NoError(t, err)
}

func TestProbeFailsWhenHealthzAnswersAnotherStatus(t *testing.T) {
	port := serveHealthz(t, http.StatusServiceUnavailable)

	err := healthcheck.Probe(t.Context(), http.DefaultClient, port)

	require.ErrorContains(t, err, "503")
}
