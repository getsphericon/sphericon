package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The event router and the job workers have no outside face: that Close stops them is checked
// by the goleak guard in main_test.go, which fails the package on a goroutine left running.

// answers reports whether url answers at all, with any status.
func answers(t *testing.T, url string) bool {
	t.Helper()
	code, _, err := testhelper.TryHTTPGet(t.Context(), url)
	return err == nil && code != 0
}

func TestStartServesAndCloseStopsEverything(t *testing.T) {
	a, base := lifecycleApp(t)
	require.NoError(t, a.Start(context.Background()))

	require.Eventually(t, func() bool {
		code, _, err := testhelper.TryHTTPGet(t.Context(), base+"/healthz")
		return err == nil && code == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond, "the public server answers once Start returned")

	require.NoError(t, a.Close(closeCtx(t)))

	closeErr := testhelper.Receive(t, a.Done(), "Done never reported")
	require.NoError(t, closeErr, "a graceful close is not a server error")
	assert.False(t, answers(t, base+"/healthz"), "the listener is closed")
}

func TestStartServesEveryAPISurfaceOnACallerProvidedListener(t *testing.T) {
	a, base := lifecycleApp(t)
	assert.Equal(t, base, a.Config.AppURL, "the public URL is derived from the listener")

	require.NoError(t, a.Start(context.Background()))
	t.Cleanup(func() { cleanup.Report("close the app", a.Close(closeCtx(t))) })

	for _, path := range []string{"/healthz", "/api/auth/me", "/site/workspaces", "/collect/events"} {
		require.Eventually(t, func() bool {
			code, _, err := testhelper.TryHTTPGet(t.Context(), base+path)
			return err == nil && code != http.StatusNotFound
		}, 5*time.Second, 20*time.Millisecond, path)
	}
}

func TestStartServesMetricsAndCloseStopsThem(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_METRICS_ADDR", freeAddr(t))
	a, err := app.New("test", app.WithListener(listen(t)))
	require.NoError(t, err)
	require.NotNil(t, a.Metrics)
	require.NoError(t, a.Start(context.Background()))
	url := "http://" + a.Metrics.Addr() + "/metrics"

	require.Eventually(t, func() bool { return answers(t, url) }, 5*time.Second, 20*time.Millisecond,
		"the metrics listener accepts once Start returned")

	require.NoError(t, a.Close(closeCtx(t)))
	assert.False(t, answers(t, url), "the metrics listener stopped accepting after Close")
}

func TestCloseIsIdempotent(t *testing.T) {
	a, _ := lifecycleApp(t)
	require.NoError(t, a.Start(context.Background()))
	require.NoError(t, a.Close(closeCtx(t)))
	assert.NoError(t, a.Close(closeCtx(t)))
}

func TestStartTwiceIsRefused(t *testing.T) {
	a, _ := lifecycleApp(t)
	require.NoError(t, a.Start(context.Background()))

	require.ErrorIs(t, a.Start(context.Background()), app.ErrAlreadyStarted)

	require.NoError(t, a.Close(closeCtx(t)))
}

func TestCloseStopsAnAppWhoseStartContextWasCancelled(t *testing.T) {
	a, _ := lifecycleApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, a.Start(ctx))
	cancel()
	require.NoError(t, a.Close(closeCtx(t)))
}

func TestStartUnwindsWhenTheMetricsAddressIsTaken(t *testing.T) {
	baseline(t)
	taken := listen(t)
	t.Cleanup(func() { cleanup.Close(taken, "taken") })
	t.Setenv("SPHERICON_METRICS_ADDR", taken.Addr().String())
	ln := listen(t)
	a, err := app.New("test", app.WithListener(ln))
	require.NoError(t, err)

	require.Error(t, a.Start(context.Background()))

	assert.False(t, answers(t, "http://"+ln.Addr().String()+"/healthz"), "nothing serves after a failed Start")
	assert.NotNil(t, a.Shutdown(context.Background()), "the container is already shut down")
	assert.NoError(t, a.Close(closeCtx(t)), "Close after a failed Start is harmless")
}
