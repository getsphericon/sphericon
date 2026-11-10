package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/maypok86/otter/v2"
)

// How long a verdict is trusted before the database is asked again: a schema that is current
// is rechecked rarely, one that is behind quickly, so a `migrate` takes effect within a second.
const (
	currentTTL = 10 * time.Second
	behindTTL  = time.Second
)

// checkTimeout bounds a check, which runs apart from the request that started it and so has no
// other bound: a database that does not answer fails the check, as it fails the readiness probe
// in as long. The error is not cached, so the next request asks again.
const checkTimeout = 2 * time.Second

// SchemaWatch answers, for a process that is already serving, how many of the migrations this
// binary embeds the database has not run (a reset or a restored backup under a running dev
// stack, after the start guard long passed). The answer is cached, and concurrent callers share
// one check, so asking per request costs one lookup every few seconds.
type SchemaWatch struct {
	check   func(ctx context.Context) (int, error)
	verdict *otter.Cache[struct{}, int]
}

// NewSchemaWatch watches sqlDB against the embedded migrations, on the Instance clock now.
func NewSchemaWatch(sqlDB *sql.DB, now func() time.Time) *SchemaWatch {
	return WatchSchema(func(ctx context.Context) (int, error) {
		return pendingMigrations(ctx, sqlDB)
	}, now)
}

// WatchSchema is NewSchemaWatch over any count of pending migrations and any clock.
func WatchSchema(check func(ctx context.Context) (int, error), now func() time.Time) *SchemaWatch {
	verdict, err := otter.New(&otter.Options[struct{}, int]{
		ExpiryCalculator: otter.ExpiryWritingFunc(func(e otter.Entry[struct{}, int]) time.Duration {
			if e.Value > 0 {
				return behindTTL
			}
			return currentTTL
		}),
		Clock: instanceClock(now),
	})
	if err != nil {
		panic(err)
	}
	return &SchemaWatch{check: check, verdict: verdict}
}

// Forget drops the cached verdict, so the next Pending asks the database: a migration run by
// this process takes effect on the next request. A check in flight is discarded and run again.
func (w *SchemaWatch) Forget() {
	w.verdict.Invalidate(struct{}{})
}

// Pending is the number of migrations the database has not run. An error is not cached: the
// next call asks again. The check is shared by every caller waiting for it, so it must not end
// with the request that happened to start it: it runs without that request's cancellation,
// within checkTimeout.
func (w *SchemaWatch) Pending(ctx context.Context) (int, error) {
	return w.verdict.Get(ctx, struct{}{}, otter.LoaderFunc[struct{}, int](
		func(ctx context.Context, _ struct{}) (int, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), checkTimeout)
			defer cancel()
			return w.check(ctx)
		}))
}

// instanceClock runs the verdict's expiry on the Instance clock. It gives no ticks: a periodic
// clean-up would take a goroutine of its own that a custom clock cannot stop, and the one
// verdict is checked for expiry whenever it is read.
type instanceClock func() time.Time

func (c instanceClock) NowNano() int64 { return c().UnixNano() }

func (instanceClock) Tick(time.Duration) <-chan time.Time { return nil }
