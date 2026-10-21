package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/getsphericon/sphericon/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const otelScope = "github.com/getsphericon/sphericon/internal/db"

// Pool label values: the only label on the pool gauges, so cardinality is
// bounded by the number of pools (two).
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

// RegisterPoolMetrics exports both pools' statistics as OTel observable gauges
// on the global meter provider (so they surface wherever that provider is
// scraped), labelled only by pool name. Either pool may be nil.
func RegisterPoolMetrics(sqlDB, riverDB *sql.DB) (metric.Registration, error) {
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
	waitCount := mk("wait_count", "Cumulative number of connection requests that had to wait (sql.DBStats.WaitCount).")
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for pool, d := range map[string]*sql.DB{PoolSQL: sqlDB, PoolRiver: riverDB} {
			if d == nil {
				continue
			}
			st := d.Stats()
			a := metric.WithAttributes(attribute.String("pool", pool))
			o.ObserveInt64(open, int64(st.OpenConnections), a)
			o.ObserveInt64(inUse, int64(st.InUse), a)
			o.ObserveInt64(idle, int64(st.Idle), a)
			o.ObserveInt64(maxOpen, int64(st.MaxOpenConnections), a)
			o.ObserveInt64(waitCount, st.WaitCount, a)
		}
		return nil
	}, open, inUse, idle, maxOpen, waitCount)
}
