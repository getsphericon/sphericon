package testhelper

import (
	"context"
	"database/sql"
	"net/url"
	"runtime"
	"testing"

	"github.com/mokevnin/dbcleaner"
	"github.com/stretchr/testify/require"
)

// The baseline's worker is held by a session of the lease, which the server drops when the
// process ends. A lease nothing refers to is closed by the garbage collector, which hands the
// worker to another process: that one loads its fixtures into it and drops the clones of the
// baseline, mid-run. So a worker asked for after a collection is never the baseline's.
func TestBaselineWorkerStaysLeasedAfterGarbageCollection(t *testing.T) {
	base := baselineDB(t)

	for range 3 {
		runtime.GC()
	}

	version, err := schemaHash()
	require.NoError(t, err)
	other := dbcleaner.New(base.shared,
		dbcleaner.WithTemplate(version, func(ctx context.Context, sqlDB *sql.DB, _ string) error {
			return migrateSchema(ctx, sqlDB)
		}),
	)
	t.Cleanup(func() { require.NoError(t, other.Close()) })
	dsn, err := other.URL(t.Context())
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)

	require.NotEqual(t, base.name, u.Path[1:], "another process was given the baseline's worker")
}
