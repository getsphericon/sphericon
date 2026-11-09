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
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(config.EnvTest)
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

// Both pools' stats surface in a scrape of the global meter provider as otelsql's
// db.sql.connection.* metrics, told apart by the pool attribute.
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

	text := string(body)
	for name, maxOpen := range map[string]string{"sql": "7", "river": "11"} {
		assert.Regexp(t, `db_sql_connection_max_open\{[^}]*pool="`+name+`"[^}]*\} `+maxOpen, text)
		for _, status := range []string{"inuse", "idle"} {
			assert.Regexp(t, `db_sql_connection_open\{[^}]*pool="`+name+`"[^}]*status="`+status+`"[^}]*\}`, text)
		}
		assert.Regexp(t, `db_sql_connection_wait_total\{[^}]*pool="`+name+`"[^}]*\}`, text)
	}
	assert.NotContains(t, text, "db_pool_", "the hand-written gauges are gone")
}
