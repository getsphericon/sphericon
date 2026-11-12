package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"math"

	sphericon "github.com/getsphericon/sphericon"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// SchemaStatus is what `sphericon migrate --status` prints: the newest migration the
// schema has (0 on an empty database), the newest this binary knows and the migrations
// that would run, oldest first.
type SchemaStatus struct {
	Current int64
	Newest  int64
	Pending []string
	// State is the state row (Major 1 when there is none yet) and BinaryMajor the Major of
	// this binary's own schema. Status fills them; StatusFS, which has no table of Majors, does not.
	State       SchemaState
	BinaryMajor int
}

// Migrate applies the embedded migrations to sqlDB with goose and reports how many it ran: the
// app schema and river's tables come from them. It is the one
// path the server (`sphericon migrate`) and the test databases share, so a test runs against
// the schema production gets.
//
// It takes a database lock, so two migrations started together run one after the other and
// the second finds nothing to do. It has no time limit of its own: the wait for the lock and
// the run end when ctx does, so the caller chooses (`migrate` passes none). A migration that runs
// in a transaction waits for a table's lock only briefly and is retried (lockTimeout), so a
// long transaction delays the migration rather than the queries of the table.
func Migrate(ctx context.Context, sqlDB *sql.DB) (int, error) {
	files, err := embeddedMigrations()
	if err != nil {
		return 0, err
	}
	return migrateFS(ctx, sqlDB, files, goMigrations())
}

// Status reads the schema state and the embedded migrations without changing anything.
func Status(ctx context.Context, sqlDB *sql.DB) (SchemaStatus, error) {
	files, err := embeddedMigrations()
	if err != nil {
		return SchemaStatus{}, err
	}
	st, err := statusFS(ctx, sqlDB, files, goMigrations())
	if err != nil {
		return SchemaStatus{}, err
	}
	if st.State, err = ReadState(ctx, sqlDB); err != nil {
		return SchemaStatus{}, err
	}
	st.BinaryMajor = Majors[len(Majors)-1].Major
	return st, nil
}

// MigrateBefore is MigrateFS over files with the Go migrations older than version, as Migrate runs
// them among the SQL files: the schema as it stood before the migration of that version, for a
// test of that migration.
func MigrateBefore(ctx context.Context, sqlDB *sql.DB, files fs.FS, version int64) (int, error) {
	var older []*goose.Migration
	for _, m := range goMigrations() {
		if m.Version < version {
			older = append(older, m)
		}
	}
	return migrateFS(ctx, sqlDB, files, older)
}

// MigrateFS is Migrate over any migrations directory of SQL files.
func MigrateFS(ctx context.Context, sqlDB *sql.DB, files fs.FS) (int, error) {
	return migrateFS(ctx, sqlDB, files, nil)
}

func migrateFS(ctx context.Context, sqlDB *sql.DB, files fs.FS, gos []*goose.Migration) (int, error) {
	// A server older than the schema needs stops here, before a migration it cannot run (a SET NULL
	// with a column list) leaves the schema half changed.
	major, err := postgres.ServerMajor(ctx, sqlDB)
	if err != nil {
		return 0, err
	}
	if major < postgres.MinMajor {
		return 0, fmt.Errorf("PostgreSQL %d is older than %d, the oldest the schema runs on: upgrade PostgreSQL first", major, postgres.MinMajor)
	}
	// A lock wait is bounded by ctx alone: goose's own default would give up after five minutes.
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, math.MaxInt32))
	if err != nil {
		return 0, err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, lockBoundedFS{files}, goose.WithSessionLocker(locker), goose.WithGoMigrations(gos...))
	if err != nil {
		return 0, err
	}
	return upWithLockRetries(ctx, provider)
}

// StatusFS is Status over any migrations directory of SQL files.
func StatusFS(ctx context.Context, sqlDB *sql.DB, files fs.FS) (SchemaStatus, error) {
	return statusFS(ctx, sqlDB, files, nil)
}

func statusFS(ctx context.Context, sqlDB *sql.DB, files fs.FS, gos []*goose.Migration) (SchemaStatus, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files, goose.WithGoMigrations(gos...))
	if err != nil {
		return SchemaStatus{}, err
	}
	all, err := provider.Status(ctx)
	if err != nil {
		return SchemaStatus{}, err
	}
	var st SchemaStatus
	for _, m := range all {
		st.Newest = max(st.Newest, m.Source.Version)
		if m.State == goose.StateApplied {
			st.Current = max(st.Current, m.Source.Version)
		} else {
			st.Pending = append(st.Pending, m.Source.Path)
		}
	}
	return st, nil
}

func embeddedMigrations() (fs.FS, error) {
	return fs.Sub(sphericon.MigrationsFS, "migrations")
}
