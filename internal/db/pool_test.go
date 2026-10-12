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
	t.Cleanup(func() { _ = sqlDB.Close() })

	db.ConfigurePool(sqlDB, config.DBPool{MaxOpenConns: 7, MaxIdleConns: 3, ConnMaxLifetime: time.Minute, PGXMaxConns: 11})

	assert.Equal(t, 7, sqlDB.Stats().MaxOpenConnections)
}

func TestNewPGXPoolAppliesMaxConns(t *testing.T) {
	cfg := testConfig(t)
	pool, err := db.NewPGXPool(context.Background(), cfg.DatabaseURL, config.DBPool{PGXMaxConns: 11})
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	assert.Equal(t, int32(11), pool.Config().MaxConns)
	assert.Equal(t, int32(11), pool.Stat().MaxConns())
}

// Pool stats surface in a scrape of the global meter provider, labelled only
// by pool name.
func TestPoolMetricsAppearInScrape(t *testing.T) {
	cfg := testConfig(t)
	pool := config.DBPool{MaxOpenConns: 7, MaxIdleConns: 7, ConnMaxLifetime: time.Minute, PGXMaxConns: 11}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db.ConfigurePool(sqlDB, pool)
	pgxPool, err := db.NewPGXPool(context.Background(), cfg.DatabaseURL, pool)
	require.NoError(t, err)
	t.Cleanup(pgxPool.Close)

	reg := prometheus.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(reg))
	require.NoError(t, err)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev); _ = mp.Shutdown(context.Background()) })

	registration, err := db.RegisterPoolMetrics(sqlDB, pgxPool)
	require.NoError(t, err)
	t.Cleanup(func() { _ = registration.Unregister() })

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/metrics", nil))
	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)

	assert.Regexp(t, `db_pool_max\{[^}]*pool="sql"\} 7`, string(body))
	assert.Regexp(t, `db_pool_max\{[^}]*pool="pgx"\} 11`, string(body))
	assert.Contains(t, string(body), "db_pool_in_use")
	assert.Contains(t, string(body), "db_pool_idle")
	// Each wait counter is reported for its own pool only.
	assert.Regexp(t, `db_pool_wait_count\{[^}]*pool="sql"\}`, string(body))
	assert.NotRegexp(t, `db_pool_wait_count\{[^}]*pool="pgx"\}`, string(body))
	assert.Regexp(t, `db_pool_empty_acquire_count\{[^}]*pool="pgx"\}`, string(body))
	assert.NotRegexp(t, `db_pool_empty_acquire_count\{[^}]*pool="sql"\}`, string(body))
}
