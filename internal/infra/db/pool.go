package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/getsphericon/sphericon/config"
)

// Pool attribute values of the pool metrics (RegisterPoolMetrics).
const (
	PoolSQL   = "sql"
	PoolRiver = "river"
)

// ConfigurePool applies the DB_* limits to the database/sql pool so the app
// never opens unbounded Postgres connections.
func ConfigurePool(sqlDB *sql.DB, p config.DBPool) {
	sqlDB.SetMaxOpenConns(p.MaxOpenConns)
	sqlDB.SetMaxIdleConns(p.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(p.ConnMaxLifetime)
}

// OpenRiverDB opens the river pool: a database/sql pool of its own, capped at
// SPHERICON_RIVER_DB_MAX_CONNS, so job workers can never starve the API's pool.
func OpenRiverDB(url string, p config.DBPool) (*sql.DB, error) {
	riverDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	riverDB.SetMaxOpenConns(int(p.RiverMaxConns))
	riverDB.SetMaxIdleConns(int(p.RiverMaxConns))
	riverDB.SetConnMaxLifetime(p.ConnMaxLifetime)
	return riverDB, nil
}

// OpenRiverListener opens the pool river LISTENs on for job notifications. River takes its one
// connection for good (it is never returned), so the pool is capped at one per client. LISTEN
// needs a session of its own: behind PgBouncer the URL must reach Postgres directly or a pool in
// session mode, never transaction mode.
func OpenRiverListener(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 1
	return pgxpool.NewWithConfig(ctx, cfg)
}

// RegisterPoolMetrics exports both pools' statistics through otelsql (the db.sql.connection.*
// metrics) on the global meter provider, so they surface wherever that provider is scraped. The
// pool attribute is their one label of ours, so cardinality is bounded by the number of pools
// (two). The function it returns unregisters both.
func RegisterPoolMetrics(sqlDB, riverDB *sql.DB) (func() error, error) {
	sqlReg, err := registerPoolMetrics(PoolSQL, sqlDB)
	if err != nil {
		return nil, err
	}
	riverReg, err := registerPoolMetrics(PoolRiver, riverDB)
	if err != nil {
		return nil, errors.Join(err, sqlReg.Unregister())
	}
	return func() error { return errors.Join(sqlReg.Unregister(), riverReg.Unregister()) }, nil
}

func registerPoolMetrics(pool string, d *sql.DB) (metric.Registration, error) {
	reg, err := otelsql.RegisterDBStatsMetrics(d, otelsql.WithAttributes(attribute.String("pool", pool)))
	if err != nil {
		return nil, fmt.Errorf("register the %s pool metrics: %w", pool, err)
	}
	return reg, nil
}
