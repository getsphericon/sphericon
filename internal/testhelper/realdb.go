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
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/peterldowns/pgtestdb"
	"github.com/stretchr/testify/require"
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

// EmptyDBURL is the URL of a test's own database with no schema at all, for code that
// migrates a fresh instance (RealDB starts at the fixture state).
func EmptyDBURL(t *testing.T) string {
	t.Helper()
	return pgtestdb.Custom(t, serverConfig(t), emptyMigrator{}).URL()
}

// serverConfig points pgtestdb at the test server from the same DATABASE_URL the
// application reads in the test environment.
func serverConfig(t *testing.T) pgtestdb.Config {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	u, err := url.Parse(cfg.DatabaseURL)
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
	return hashTrees(projectRoot(),
		"ent/schema",
		"fixtures",
		"internal/events/bus.go",
		"go.mod",
	)
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
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// schemaHash digests everything that decides the schema a template holds.
func schemaHash() (string, error) {
	return hashTrees(projectRoot(), "ent/schema", "internal/events/bus.go", "go.mod")
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
			name, _ := filepath.Rel(root, path)
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
