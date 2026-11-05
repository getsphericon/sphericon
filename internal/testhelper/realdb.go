package testhelper

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mokevnin/dbcleaner"
	dbcleanerpg "github.com/mokevnin/dbcleaner/postgres"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// RealDB is a database of the test's own on the test server, at the fixture state and
// NOT inside a transaction: whatever the test commits is committed, and a second
// connection sees it. For code that needs a real commit (a river runtime, concurrent
// workers, a whole app). It is cloned from this process's baseline (see baselineDB) and
// dropped when the test ends, so nothing is cleaned up by hand. The ordinary tests ride
// go-txdb (Setup).
func RealDB(t *testing.T) *sql.DB {
	t.Helper()
	sqlDB, err := sql.Open("pgx", RealDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "RealDB") })
	return sqlDB
}

// RealDBURL is RealDB for code that opens its own connection (an app, a pool under
// test): the URL of the test's own database at the fixture state, dropped with the test.
func RealDBURL(t *testing.T) string {
	t.Helper()
	base := baselineDB(t)
	name := fmt.Sprintf("%s_c%d", base.name, cloneSeq.Add(1))
	ctx := context.WithoutCancel(t.Context())

	// The test's own connection to the shared test database, closed with it: a pool that outlived
	// the test would be a goroutine the packages that verify none are left (goleak) refuse.
	admin, err := sql.Open("pgx", base.shared)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(admin, "RealDB admin") })

	// A database cannot be cloned while another clone of the same template is being made.
	cloneMu.Lock()
	// raw sql: CREATE DATABASE ... TEMPLATE is server DDL; ent models tables, not databases.
	_, err = admin.ExecContext(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE "+pgx.Identifier{base.name}.Sanitize())
	cloneMu.Unlock()
	require.NoError(t, err, "clone the baseline database")
	t.Cleanup(func() {
		// raw sql: DROP DATABASE is server DDL; ent models tables, not databases.
		_, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		require.NoError(t, err, "drop the clone %s", name)
	})
	return withDatabase(t, base.url, name)
}

// EmptyDBURL is the URL of a test's own database with no schema at all, for code that
// migrates a fresh instance (RealDB starts at the fixture state). It is dropped with the test.
func EmptyDBURL(t *testing.T) string {
	t.Helper()
	dsn, err := sharedDatabaseURL()
	require.NoError(t, err)
	return dbcleanerpg.ScratchDatabase(t, dsn)
}

var (
	baselineOnce sync.Once
	baseline     baselineHandle
	cloneMu      sync.Mutex
	cloneSeq     atomic.Int64
)

// baselineHandle is this process's baseline database: the one every RealDB is cloned from.
type baselineHandle struct {
	name   string // the database's name
	url    string // its URL
	shared string // the URL of the shared test database, where the clones are created and dropped
	// The Cleaner that leased the worker, kept for as long as the process runs: its session holds
	// the lease, and a session nothing refers to is closed by the garbage collector, which would
	// give the worker to another process while this one still clones from it.
	lease *dbcleaner.Cleaner
	err   error
}

// baselineDB leases, on the first call of the process, a database of its own (a dbcleaner
// worker, like the one Setup's transactions run on, built from the same schema template) and
// loads the fixtures into it NOW, so their relative dates count from this process's start and
// not from when a template was built. Nothing connects to it afterwards: it is only the source
// the clones are made from, which is why a clone costs tens of milliseconds and not the half
// second the fixtures take to load. The server releases the lease when the process ends, however
// it ends; the clones a crashed process left behind are dropped by the next one that leases the
// same worker.
func baselineDB(t *testing.T) baselineHandle {
	t.Helper()
	baselineOnce.Do(func() { baseline = openBaseline() })
	require.NoError(t, baseline.err, "open the baseline database")
	return baseline
}

func openBaseline() baselineHandle {
	ctx := context.Background()
	shared, err := sharedDatabaseURL()
	if err != nil {
		return baselineHandle{err: err}
	}
	cfg, err := config.Load("test")
	if err != nil {
		return baselineHandle{err: err}
	}
	version, err := schemaHash()
	if err != nil {
		return baselineHandle{err: err}
	}
	// The Null strategy: nothing is undone between tests, a test only ever gets a clone.
	leased := dbcleaner.New(shared,
		dbcleaner.WithTemplate(version, func(ctx context.Context, sqlDB *sql.DB, _ string) error {
			return migrateSchema(ctx, sqlDB)
		}),
		dbcleaner.WithSetup(func(ctx context.Context, sqlDB *sql.DB) error {
			return loadFixtures(ctx, sqlDB, cfg, projectRoot())
		}),
		dbcleaner.WithStrategy(dbcleaner.Null()),
	)
	dsn, err := leased.URL(ctx)
	if err != nil {
		return baselineHandle{err: err}
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return baselineHandle{err: err}
	}
	name := u.Path[1:]

	admin, err := sql.Open("pgx", shared)
	if err != nil {
		return baselineHandle{err: err}
	}
	defer cleanup.Close(admin, "baseline admin")
	if err := dropClones(ctx, admin, name); err != nil {
		return baselineHandle{err: err}
	}
	return baselineHandle{name: name, url: dsn, shared: shared, lease: leased}
}

// BaselineLeaseHeld tells whether this process still holds the lease of the worker its RealDB
// clones are made from: the advisory lock dbcleaner takes on the worker's name (the FNV-64a of
// "worker:<name>", split by the server into its class and object ids), which the server drops
// with the session that took it. A test of the harness asks it after a garbage collection.
func BaselineLeaseHeld(t *testing.T) bool {
	t.Helper()
	base := baselineDB(t)
	admin, err := sql.Open("pgx", base.shared)
	require.NoError(t, err)
	defer cleanup.Close(admin, "BaselineLeaseHeld admin")

	h := fnv.New64a()
	_, err = h.Write([]byte("worker:" + base.name))
	require.NoError(t, err)
	key := h.Sum64()
	var held int
	// raw sql: pg_locks is a server view; ent has no entity for it.
	err = admin.QueryRowContext(t.Context(),
		`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND granted AND objsubid = 1 AND classid = $1::bigint::oid AND objid = $2::bigint::oid`,
		int64(key>>32), int64(key&0xffffffff)).Scan(&held)
	require.NoError(t, err)
	return held > 0
}

// dropClones drops what an earlier process left of the baseline's clones. Only the process that
// leases the baseline's worker is here, so none of them is in use.
func dropClones(ctx context.Context, admin *sql.DB, base string) error {
	stale, err := cloneNames(ctx, admin, base)
	if err != nil {
		return err
	}
	for _, name := range stale {
		// raw sql: DROP DATABASE is server DDL; ent models tables, not databases.
		if _, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			return err
		}
	}
	return nil
}

// cloneNames lists the databases that are clones of base.
func cloneNames(ctx context.Context, admin *sql.DB, base string) ([]string, error) {
	// raw sql: pg_database is a server catalog; ent has no entity for it.
	rows, err := admin.QueryContext(ctx, `SELECT datname FROM pg_database WHERE datname LIKE $1`, base+`\_c%`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// withDatabase is dsn pointed at the database name.
func withDatabase(t *testing.T, dsn, name string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + name
	return u.String()
}

// LockWaiters counts the connections to db's database that wait for a lock another transaction
// holds: a test of the order two transactions take tells by it that the second one waits for the
// first, without guessing how long it takes to get there.
// It returns an error rather than failing the test, so a condition polled off the test's
// goroutine (require.Eventually) can call it.
func LockWaiters(ctx context.Context, db *sql.DB) (int, error) {
	// raw sql: pg_stat_activity is a server view; ent has no entity for it.
	row := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'")
	var n int
	err := row.Scan(&n)
	return n, err
}

// Listeners counts the connections to db's database whose last statement was a LISTEN: a client
// that waits for notifications. Like LockWaiters it returns an error rather than failing the test.
func Listeners(ctx context.Context, db *sql.DB) (int, error) {
	// raw sql: pg_stat_activity is a server view; ent has no entity for it.
	row := db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND query ILIKE 'listen%'")
	var n int
	err := row.Scan(&n)
	return n, err
}

// sharedDatabaseURL is the test database every process and every checkout run shares, as
// SPHERICON_DATABASE_URL names it before initBaseline points the process at its own leased database.
// It is read once, so it never depends on whether a test in the process has called Setup yet.
var sharedDatabaseURL = sync.OnceValues(func() (string, error) {
	cfg, err := config.Load("test")
	if err != nil {
		return "", err
	}
	return cfg.DatabaseURL, nil
})

func projectRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testhelper: cannot locate its own source to find the project root")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// schemaHash digests everything that decides the schema a template holds: the migrations, which
// are the only thing that creates it (an ent/schema edit shows up here once `mise run
// db:generate` has written its migration).
func schemaHash() (string, error) {
	return hashTrees(projectRoot(), "migrations")
}

// hashTrees digests the names (relative to root, so a checkout's location does not
// matter) and contents of the given files and directories.
func hashTrees(root string, paths ...string) (string, error) {
	h := sha256.New()
	for _, rel := range paths {
		err := filepath.WalkDir(filepath.Join(root, rel), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			name, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			h.Write([]byte(name))
			h.Write(data)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
