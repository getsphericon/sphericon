package db_test

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	return cfg
}

func TestConfigurePoolAppliesLimits(t *testing.T) {
	cfg := testConfig(t)
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })

	db.ConfigurePool(sqlDB, config.DBPool{MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetime: time.Minute, RiverMaxConns: 11})

	assert.Equal(t, 7, sqlDB.Stats().MaxOpenConnections)
}

func TestOpenRiverDBAppliesMaxConns(t *testing.T) {
	cfg := testConfig(t)
	riverDB, err := db.OpenRiverDB(cfg.DatabaseURL, config.DBPool{RiverMaxConns: 11, ConnMaxLifetime: time.Minute})
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(riverDB, "riverDB") })

	assert.Equal(t, 11, riverDB.Stats().MaxOpenConnections)
}

// Pool stats surface in a scrape of the global meter provider, labelled only
// by pool name.
func TestPoolMetricsAppearInScrape(t *testing.T) {
	cfg := testConfig(t)
	pool := config.DBPool{MaxOpenConns: 7, MaxIdleConns: 7, ConnMaxLifetime: time.Minute, RiverMaxConns: 11}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	db.ConfigurePool(sqlDB, pool)
	riverDB, err := db.OpenRiverDB(cfg.DatabaseURL, pool)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(riverDB, "riverDB") })

	reg := prometheus.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(reg))
	require.NoError(t, err)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev); cleanup.Report("shut mp down", mp.Shutdown(context.Background())) })

	registration, err := db.RegisterPoolMetrics(sqlDB, riverDB)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Report("unregister the gauge", registration.Unregister()) })

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)

	assert.Regexp(t, `db_pool_max\{[^}]*pool="sql"\} 7`, string(body))
	assert.Regexp(t, `db_pool_max\{[^}]*pool="river"\} 11`, string(body))
	assert.Contains(t, string(body), "db_pool_in_use")
	assert.Contains(t, string(body), "db_pool_idle")
	assert.Regexp(t, `db_pool_wait_count\{[^}]*pool="sql"\}`, string(body))
	assert.Regexp(t, `db_pool_wait_count\{[^}]*pool="river"\}`, string(body))
}
