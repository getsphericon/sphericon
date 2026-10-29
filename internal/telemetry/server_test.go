package telemetry_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/telemetry"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestMetricsServerServesExpositionOnItsOwnListener(t *testing.T) {
	testhelper.InstallOtel(t, func(ctx context.Context) (func(context.Context) error, error) {
		return telemetry.Setup(ctx, &config.Config{OtelServiceName: "sphericon-test"}, "test", telemetry.BuildInfo{})
	})

	srv := telemetry.NewMetricsServer("127.0.0.1:0")
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
