package db

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// How long a verdict is trusted before the database is asked again: a schema that is current
// is rechecked rarely, one that is behind quickly, so a `migrate` takes effect within a second.
const (
	currentTTL = 10 * time.Second
	behindTTL  = time.Second
)

// SchemaWatch answers, for a process that is already serving, how many of the migrations this
// binary embeds the database has not run (a reset or a restored backup under a running dev
// stack, after the start guard long passed). The answer is cached, so asking per request
// costs one lookup every few seconds.
type SchemaWatch struct {
	check func(ctx context.Context) (int, error)
	now   func() time.Time

	mu      sync.Mutex
	pending int
	until   time.Time
}

// NewSchemaWatch watches sqlDB against the embedded migrations.
func NewSchemaWatch(sqlDB *sql.DB) *SchemaWatch {
	return WatchSchema(func(ctx context.Context) (int, error) {
		return pendingMigrations(ctx, sqlDB)
	}, time.Now)
}

// WatchSchema is NewSchemaWatch over any count of pending migrations and any clock.
func WatchSchema(check func(ctx context.Context) (int, error), now func() time.Time) *SchemaWatch {
	return &SchemaWatch{check: check, now: now}
}

// Pending is the number of migrations the database has not run. An error is not cached: the
// next call asks again.
func (w *SchemaWatch) Pending(ctx context.Context) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.now().Before(w.until) {
		return w.pending, nil
	}
	pending, err := w.check(ctx)
	if err != nil {
		return 0, err
	}
	ttl := currentTTL
	if pending > 0 {
		ttl = behindTTL
	}
	w.pending = pending
	w.until = w.now().Add(ttl)
	return pending, nil
}
