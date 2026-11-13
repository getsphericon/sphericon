package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3/lock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func openEmpty(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
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

// Migrate has no time limit of its own: it applies a migration under a context with no
// deadline, and the lock wait is bounded by that context alone.
func TestMigrateFSAppliesAMigrationUnderAContextWithNoDeadline(t *testing.T) {
	sqlDB := openEmpty(t)
	files := fstest.MapFS{"20200101000000_slow.sql": {Data: []byte("-- +goose Up\nCREATE TABLE slow_done (id int);\n")}}
	_, hasDeadline := t.Context().Deadline()
	require.False(t, hasDeadline)

	applied, err := db.MigrateFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Equal(t, 1, applied)

	st, err := db.StatusFS(t.Context(), sqlDB, files)
	require.NoError(t, err)
	assert.Empty(t, st.Pending)
}

// waitForLockWait waits until a query of the database waits for a lock.
func waitForLockWait(t *testing.T, sqlDB *sql.DB) {
	t.Helper()
	require.Eventually(t, func() bool {
		n, err := testhelper.LockWaiters(t.Context(), sqlDB)
		return err == nil && n > 0
	}, 10*time.Second, 20*time.Millisecond)
}

// migrating runs the migrations in the background; the channel receives MigrateFS's error.
func migrating(t *testing.T, sqlDB *sql.DB, files fstest.MapFS) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := db.MigrateFS(t.Context(), sqlDB, files)
		done <- err
	}()
	return done
}

var tableMigration = fstest.MapFS{"20200101000001_table.sql": {Data: []byte("-- +goose Up\nCREATE TABLE busy (id int);\n")}}

func withMigration(name, src string) fstest.MapFS {
	files := maps.Clone(tableMigration)
	files[name] = &fstest.MapFile{Data: []byte(src)}
	return files
}

// A migration that waits behind a long transaction for its table's lock gives up after a short
// wait and is retried, so the table's queries are not queued behind it until the transaction
// ends, and it applies once the transaction does.
func TestMigrateFSRetriesALockWaitWithoutStallingTheTablesQueries(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.MigrateFS(t.Context(), sqlDB, tableMigration)
	require.NoError(t, err)
	commit := testhelper.HoldInTransaction(t, sqlDB, "SELECT count(*) FROM busy")

	files := withMigration("20200101000002_column.sql", "-- +goose Up\nALTER TABLE busy ADD COLUMN note text;\n")
	done := migrating(t, sqlDB, files)
	waitForLockWait(t, sqlDB)

	// Without a limit on the wait this read would queue behind the migration until the commit.
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	_, err = testhelper.CountRows(ctx, sqlDB, "busy")
	require.NoError(t, err)

	commit()
	require.NoError(t, <-done)
	assert.Equal(t, 1, testhelper.ColumnsOf(t, sqlDB, "busy", "note"))
}

// A migration whose lock stays taken until its context ends fails naming the migration.
func TestMigrateFSNamesTheMigrationWhoseLockStayedTaken(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.MigrateFS(t.Context(), sqlDB, tableMigration)
	require.NoError(t, err)
	testhelper.HoldInTransaction(t, sqlDB, "SELECT count(*) FROM busy")

	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	_, err = db.MigrateFS(ctx, sqlDB, withMigration("20200101000002_column.sql", "-- +goose Up\nALTER TABLE busy ADD COLUMN note text;\n"))
	require.ErrorContains(t, err, "20200101000002")
}

// A migration without a transaction keeps an unbounded wait: an index built CONCURRENTLY waits
// for the transactions writing to its table, and a limit would fail it and leave the index invalid.
func TestMigrateFSLetsANoTransactionMigrationWaitForOlderTransactions(t *testing.T) {
	sqlDB := openEmpty(t)
	_, err := db.MigrateFS(t.Context(), sqlDB, tableMigration)
	require.NoError(t, err)
	commit := testhelper.HoldInTransaction(t, sqlDB, "INSERT INTO busy VALUES (1)")

	files := withMigration("20200101000002_index.sql",
		"-- +goose NO TRANSACTION\n-- +goose Up\nCREATE INDEX CONCURRENTLY busy_id ON busy (id);\n")
	done := migrating(t, sqlDB, files)
	waitForLockWait(t, sqlDB)
	assert.Never(t, func() bool { return len(done) > 0 }, 3*time.Second, 50*time.Millisecond,
		"the build still waits past the limit a transaction's wait has")
	commit()

	require.NoError(t, <-done)
	assert.True(t, testhelper.IndexIsValid(t, sqlDB, "busy_id"))
}

// A migration that finds the migration lock taken reports who holds it while it waits, and once
// the holder lets go it says it took the lock and applies.
func TestMigrateFSReportsWhoHoldsTheMigrationLockWhileItWaits(t *testing.T) {
	sqlDB := openEmpty(t)
	logs := testhelper.CaptureLogs(t)
	holder, release := testhelper.HoldAdvisoryLock(t, sqlDB, lock.DefaultLockID)
	holderAttr := fmt.Sprintf(" holder_pid=%d ", holder)

	done := migrating(t, sqlDB, tableMigration)
	require.Eventually(t, func() bool {
		logged := logs.String()
		return strings.Contains(logged, holderAttr)
	}, 10*time.Second, 50*time.Millisecond)
	waiting := logs.String()
	assert.NotContains(t, waiting, "took the migration lock")

	release()
	err := <-done
	require.NoError(t, err)
	taken := logs.String()
	assert.Contains(t, taken, "took the migration lock")
	columns := testhelper.ColumnsOf(t, sqlDB, "busy", "id")
	assert.Equal(t, 1, columns)
}
