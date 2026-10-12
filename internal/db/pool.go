package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/getsphericon/sphericon/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const otelScope = "github.com/getsphericon/sphericon/internal/db"

// Pool label values: the only label on the pool gauges, so cardinality is
// bounded by the number of pools (two).
const (
	PoolSQL = "sql"
	PoolPGX = "pgx"
)

// ConfigurePool applies the DB_* limits to the database/sql pool so the app
// never opens unbounded Postgres connections.
func ConfigurePool(sqlDB *sql.DB, p config.DBPool) {
	sqlDB.SetMaxOpenConns(p.MaxOpenConns)
	sqlDB.SetMaxIdleConns(p.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(p.ConnMaxLifetime)
}

// NewPGXPool opens the river pool capped at PGX_MAX_CONNS.
func NewPGXPool(ctx context.Context, url string, p config.DBPool) (*pgxpool.Pool, error) {
	pgxCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pgxCfg.MaxConns = p.PGXMaxConns
	return pgxpool.NewWithConfig(ctx, pgxCfg)
}

// RegisterPoolMetrics exports both pools' statistics as OTel observable gauges
// on the global meter provider (so they surface wherever that provider is
// scraped), labelled only by pool name. Either pool may be nil.
func RegisterPoolMetrics(sqlDB *sql.DB, pgxPool *pgxpool.Pool) (metric.Registration, error) {
	meter := otel.Meter(otelScope)
	var errs []error
	mk := func(name, desc string) metric.Int64ObservableGauge {
		g, err := meter.Int64ObservableGauge("db.pool."+name, metric.WithDescription(desc))
		errs = append(errs, err)
		return g
	}
	open := mk("open", "Connections currently open (in use plus idle).")
	inUse := mk("in_use", "Connections currently in use.")
	idle := mk("idle", "Idle connections.")
	maxOpen := mk("max", "Configured maximum number of connections.")
	// Two pools, two driver-native counters: they are not the same quantity, so
	// each has its own instrument and is observed for its own pool only.
	// database/sql counts requests that blocked for a connection (WaitCount).
	waitCount := mk("wait_count", "database/sql pool only: cumulative number of connection requests that had to wait (sql.DBStats.WaitCount).")
	emptyAcquire := mk("empty_acquire_count", "pgx pool only: cumulative number of successful acquires that found the pool empty and waited for a connection to be released or opened (pgxpool.Stat.EmptyAcquireCount).")
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if sqlDB != nil {
			s := sqlDB.Stats()
			a := metric.WithAttributes(attribute.String("pool", PoolSQL))
			o.ObserveInt64(open, int64(s.OpenConnections), a)
			o.ObserveInt64(inUse, int64(s.InUse), a)
			o.ObserveInt64(idle, int64(s.Idle), a)
			o.ObserveInt64(maxOpen, int64(s.MaxOpenConnections), a)
			o.ObserveInt64(waitCount, s.WaitCount, a)
		}
		if pgxPool != nil {
			s := pgxPool.Stat()
			a := metric.WithAttributes(attribute.String("pool", PoolPGX))
			o.ObserveInt64(open, int64(s.TotalConns()), a)
			o.ObserveInt64(inUse, int64(s.AcquiredConns()), a)
			o.ObserveInt64(idle, int64(s.IdleConns()), a)
			o.ObserveInt64(maxOpen, int64(s.MaxConns()), a)
			o.ObserveInt64(emptyAcquire, s.EmptyAcquireCount(), a)
		}
		return nil
	}, open, inUse, idle, maxOpen, waitCount, emptyAcquire)
}
