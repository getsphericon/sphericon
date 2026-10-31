package testhelper

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/peterldowns/pgtestdb"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
)

// RealDB is a database of the test's own on the test server, at the fixture state and
// NOT inside a transaction: whatever the test commits is committed, and a second
// connection sees it. For code that needs a real commit (the outbox router, concurrent
// workers, a whole app). pgtestdb clones it from a template and drops it when the test
// ends, so nothing is cleaned up by hand. The ordinary tests ride go-txdb (Setup).
func RealDB(t *testing.T) *sql.DB {
	t.Helper()
	return pgtestdb.New(t, serverConfig(t), baselineMigrator{})
}

// RealDBURL is RealDB for code that opens its own connection (an app, a pool under
// test): the URL of the test's own database at the fixture state, dropped with the test.
func RealDBURL(t *testing.T) string {
	t.Helper()
	return pgtestdb.Custom(t, serverConfig(t), baselineMigrator{}).URL()
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

// EmptyDBURL is the URL of a test's own database with no schema at all, for code that
// migrates a fresh instance (RealDB starts at the fixture state).
func EmptyDBURL(t *testing.T) string {
	t.Helper()
	return pgtestdb.Custom(t, serverConfig(t), emptyMigrator{}).URL()
}

// sharedDatabaseURL is the test database every process and every checkout run shares, as
// DATABASE_URL names it before initBaseline points the process at its own leased database.
// It is read once, so it never depends on whether a test in the process has called Setup yet.
var sharedDatabaseURL = sync.OnceValues(func() (string, error) {
	cfg, err := config.Load("test")
	if err != nil {
		return "", err
	}
	return cfg.DatabaseURL, nil
})

// serverConfig points pgtestdb at the shared test database. It must not be the process's
// own database (the `_w<i>` one DATABASE_URL names once Setup has run): pgtestdb serializes
// the creation of a template with a Postgres advisory lock, and advisory locks are scoped to
// one database, so processes connected to different databases never excluded each other, and
// one dropped the template another was still building or cloning.
func serverConfig(t *testing.T) pgtestdb.Config {
	t.Helper()
	dsn, err := sharedDatabaseURL()
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	password, _ := u.User.Password()
	return pgtestdb.Config{
		DriverName: "pgx",
		Host:       u.Hostname(),
		Port:       u.Port(),
		User:       u.User.Username(),
		Password:   password,
		Database:   u.Path[1:],
		Options:    u.RawQuery,
		// The fixture loader disables foreign-key triggers, which takes a superuser, as
		// it does on the shared test database.
		TestRole: &pgtestdb.Role{
			Username:     "sphericon_pgtestdb",
			Password:     pgtestdb.DefaultRolePassword,
			Capabilities: "SUPERUSER",
		},
		ForceTerminateConnections: true,
	}
}

// baselineMigrator builds the template: the same state initBaseline gives each test
// process. Its hash covers every input of that state, so a schema or fixture change
// builds a new template and the old one is never reused.
type baselineMigrator struct{}

func (baselineMigrator) Hash() (string, error) {
	return hashTrees(projectRoot(), "migrations", "fixtures")
}

func (baselineMigrator) Migrate(ctx context.Context, sqlDB *sql.DB, conf pgtestdb.Config) error {
	cfg, err := config.Load("test")
	if err != nil {
		return err
	}
	cfg.DatabaseURL = conf.URL()
	return migrateBaseline(ctx, sqlDB, cfg, projectRoot())
}

// emptyMigrator leaves the template empty.
type emptyMigrator struct{}

func (emptyMigrator) Hash() (string, error) { return "empty", nil }

func (emptyMigrator) Migrate(context.Context, *sql.DB, pgtestdb.Config) error { return nil }

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
