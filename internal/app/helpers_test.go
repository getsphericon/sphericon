package app_test

import (
	"context"
	"database/sql"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// baseline points the app at a database of the test's own, at the fixture state, and returns its
// URL: the app connects directly and commits for real, outside the txdb harness every other
// package uses. The database is dropped with the test.
func baseline(t *testing.T) string {
	t.Helper()
	url := testhelper.RealDBURL(t)
	t.Setenv("DATABASE_URL", url)
	return url
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

// newCommandApp builds the command app on the test's own database and returns it with a client of
// the test's own on that database, to read what the app committed.
func newCommandApp(t *testing.T) (*app.App, *ent.Client) {
	t.Helper()
	url := baseline(t)
	a, err := app.NewCommandApp("test")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Shutdown(a.Shutdown(context.Background())) })
	return a, entClientOn(t, url)
}

// entClientOn opens an ent client of the test's own on the database at url.
func entClientOn(t *testing.T, url string) *ent.Client {
	t.Helper()
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	client := db.NewEntClient(sqlDB)
	t.Cleanup(func() { cleanup.Close(client, "the test's ent client") })
	return client
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

// freePort returns a loopback port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	ln := listen(t)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}
