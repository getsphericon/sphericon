package db_test

import (
	"database/sql"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func openEmpty(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

// Two migrations started together run one after the other: the second finds nothing to do.
func TestMigrateStartedTwiceTogetherRunsOnceAndTheSecondFindsNothing(t *testing.T) {
	sqlDB := openEmpty(t)

	var wg sync.WaitGroup
	applied := make([]int, 2)
	errs := make([]error, 2)
	for i := range applied {
		wg.Go(func() { applied[i], errs[i] = db.Migrate(t.Context(), sqlDB) })
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.Zero(t, min(applied[0], applied[1]))
	assert.Positive(t, max(applied[0], applied[1]))

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
	assert.Equal(t, st.Newest, st.Current)
}

// Status reads the schema and changes nothing.
func TestStatusOnAnEmptyDatabaseListsEveryMigrationAndLeavesItEmpty(t *testing.T) {
	sqlDB := openEmpty(t)

	st, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Zero(t, st.Current)
	assert.Positive(t, st.Newest)
	assert.NotEmpty(t, st.Pending)

	again, err := db.Status(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Equal(t, st, again)

	applied, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	assert.Len(t, st.Pending, applied)
}

// Migrate has no time limit of its own: a slow migration completes when the caller sets none.
func TestMigrateFSRunsASlowMigrationToCompletion(t *testing.T) {
	sqlDB := openEmpty(t)
	files := fstest.MapFS{"20200101000000_slow.sql": {Data: []byte("-- +goose Up\nSELECT pg_sleep(2);\nCREATE TABLE slow_done (id int);\n")}}

	start := time.Now()
	applied, err := db.MigrateFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)
	assert.GreaterOrEqual(t, time.Since(start), 2*time.Second)

	st, err := db.StatusFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
}
