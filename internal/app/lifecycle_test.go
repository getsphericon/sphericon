package app

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

// lifecycleApp builds an e2e-profile app on a caller-opened listener with river's
// schema in place, so Start can bring every part up.
func lifecycleApp(t *testing.T) (*App, string) {
	t.Helper()
	baseline(t)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	a, err := New("e2e", WithListener(ln))
	require.NoError(t, err)
	return a, "http://" + ln.Addr().String()
}

func closeCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestStartServesAndCloseStopsEverything(t *testing.T) {
	a, base := lifecycleApp(t)
	require.NoError(t, a.Start(context.Background()))

	require.Eventually(t, func() bool {
		code, _, err := testhelper.TryHTTPGet(t.Context(), base+"/healthz")
		return err == nil && code == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond, "the public server answers once Start returned")
	assert.True(t, a.events.router.IsRunning(), "the event router runs once Start returned")

	require.NoError(t, a.Close(closeCtx(t)))

	assert.True(t, a.events.router.IsClosed(), "Close stops the event router")
	select {
	case <-a.jobs.Stopped():
	default:
		t.Fatal("Close left the job workers running")
	}
	select {
	case err := <-a.Done():
		assert.NoError(t, err, "a graceful close is not a server error")
	case <-time.After(5 * time.Second):
		t.Fatal("Done never reported")
	}
	_, _, err := testhelper.TryHTTPGet(t.Context(), base+"/healthz")
	assert.Error(t, err, "the listener is closed")
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

	assert.ErrorIs(t, a.Start(context.Background()), ErrAlreadyStarted)

	require.NoError(t, a.Close(closeCtx(t)))
}

func TestStartCancelsWhenTheCallerContextIsCancelled(t *testing.T) {
	a, _ := lifecycleApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, a.Start(ctx))
	cancel()
	require.NoError(t, a.Close(closeCtx(t)))
	assert.True(t, a.events.router.IsClosed())
}

func TestStartUnwindsWhenTheMetricsAddressIsTaken(t *testing.T) {
	baseline(t)
	taken, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = taken.Close() })
	t.Setenv("METRICS_ADDR", taken.Addr().String())
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	a, err := New("test", WithListener(ln))
	require.NoError(t, err)

	require.Error(t, a.Start(context.Background()))

	_, _, err = testhelper.TryHTTPGet(t.Context(), "http://"+ln.Addr().String()+"/healthz")
	assert.Error(t, err, "nothing serves after a failed Start")
	assert.NotNil(t, a.Shutdown(context.Background()), "the container is already shut down")
	assert.NoError(t, a.Close(closeCtx(t)), "Close after a failed Start is harmless")
}

func TestStartUnwindsWhenTheJobQueueCannotStart(t *testing.T) {
	baseline(t)
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	a, err := New("e2e", WithListener(ln))
	require.NoError(t, err)
	// river pings the database on start; a closed pool makes that fail after the
	// event router is already running.
	pool, err := do.Invoke[*pgxPool](a.injector)
	require.NoError(t, err)
	pool.Close()

	require.Error(t, a.Start(context.Background()))

	assert.True(t, a.events.router.IsClosed(), "the router started before the failure is stopped again")
	select {
	case <-a.jobs.Stopped():
	default:
		t.Fatal("the job queue that failed to start is not reported stopped")
	}
	_, _, err = testhelper.TryHTTPGet(t.Context(), "http://"+ln.Addr().String()+"/healthz")
	assert.Error(t, err, "the public server was never left running")
}
