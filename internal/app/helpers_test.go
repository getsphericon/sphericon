package app_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// baseline points the app at a database of the test's own, at the fixture state: the app connects
// directly and commits for real, outside the txdb harness every other package uses. The database
// is dropped with the test.
func baseline(t *testing.T) {
	t.Helper()
	t.Setenv("SPHERICON_DATABASE_URL", testhelper.RealDBURL(t))
}

// newApp builds a test-profile app on the test's own database and shuts it down with the test.
func newApp(t *testing.T) *app.App {
	t.Helper()
	baseline(t)
	a, err := app.New("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	return a
}

// listen opens a loopback listener on a free port.
func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	return ln
}

// freeAddr returns a loopback host:port that was free a moment ago (config rejects
// port 0, so the app cannot be handed an ephemeral one).
func freeAddr(t *testing.T) string {
	t.Helper()
	ln := listen(t)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

// lifecycleApp builds an e2e-profile app on a caller-opened listener, so Start can bring every
// part up, and returns it with the base URL it serves on.
func lifecycleApp(t *testing.T) (a *app.App, base string) {
	t.Helper()
	baseline(t)
	ln := listen(t)
	a, err := app.New("e2e", app.WithListener(ln))
	require.NoError(t, err)
	return a, "http://" + ln.Addr().String()
}

// closeCtx bounds one Close.
func closeCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), app.StopTimeout)
	t.Cleanup(cancel)
	return ctx
}

// inProcess sends a client's requests straight to a handler, without a socket.
type inProcess struct{ handler http.Handler }

func (c inProcess) Do(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// bearer is the fixture token that carries every scope.
type bearer struct{}

func (bearer) BearerAuth(context.Context, externalapi.OperationName) (externalapi.BearerAuth, error) {
	return externalapi.BearerAuth{Token: credentials.TokenValue(fixtures.ContractTokenPrefix, fixtures.ContractTokenSecret)}, nil
}

// externalClient is the app's /api, called in process with the fixture token that carries every
// scope.
func externalClient(t *testing.T, a *app.App) *externalapi.Client {
	t.Helper()
	client, err := externalapi.NewClient("http://local/api", bearer{}, externalapi.WithClient(inProcess{a.Server.Handler}))
	require.NoError(t, err)
	return client
}
