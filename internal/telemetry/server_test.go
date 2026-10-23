package telemetry

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestMetricsServerServesExpositionOnItsOwnListener(t *testing.T) {
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", BuildInfo{})
	})

	srv := NewMetricsServer("127.0.0.1:0")
	require.NoError(t, srv.Listen(t.Context()))
	go func() { cleanup.Served("srv", srv.Serve()) }()
	t.Cleanup(func() { cleanup.Report("shut srv down", srv.Shutdown(context.Background())) })
	base := "http://" + srv.Addr()

	code, body := testhelper.HTTPGet(t, base+"/metrics")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "go_goroutine_count")

	// Tenant-label policy: see TestMetricsExpositionCarriesNoTenantLabels.
	code, _ = testhelper.HTTPGet(t, base+"/healthz")
	assert.Equal(t, http.StatusNotFound, code, "only /metrics is served")
}

func TestMetricsServerAddrIsEmptyBeforeListen(t *testing.T) {
	assert.Empty(t, NewMetricsServer("127.0.0.1:0").Addr())
}

func TestMetricsServerListenFailsOnOccupiedAddress(t *testing.T) {
	occupied, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(occupied, "occupied") })

	srv := NewMetricsServer(occupied.Addr().String())
	require.Error(t, srv.Listen(t.Context()))
}

func TestMetricsServerShutdownStopsServing(t *testing.T) {
	srv := NewMetricsServer("127.0.0.1:0")
	require.NoError(t, srv.Listen(t.Context()))
	done := make(chan error, 1)
	go func() { done <- srv.Serve() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))
	require.NoError(t, <-done, "Serve returns nil after a graceful shutdown")
}
