package db

import (
	"context"
	"database/sql"
	"io/fs"
	"math"

	sphericon "github.com/getsphericon/sphericon"
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
// app schema, river's tables and the domain-event outbox all come from them. It is the one
// path the server (`sphericon migrate`) and the test databases share, so a test runs against
// the schema production gets.
//
// It takes a database lock, so two migrations started together run one after the other and
// the second finds nothing to do. It has no time limit of its own: the wait for the lock and
// the run end when ctx does, so the caller chooses (`migrate` passes none).
func Migrate(ctx context.Context, sqlDB *sql.DB) (int, error) {
	files, err := embeddedMigrations()
	if err != nil {
		return 0, err
	}
	return MigrateFS(ctx, sqlDB, files)
}

// Status reads the schema state and the embedded migrations without changing anything.
func Status(ctx context.Context, sqlDB *sql.DB) (SchemaStatus, error) {
	files, err := embeddedMigrations()
	if err != nil {
		return SchemaStatus{}, err
	}
	st, err := StatusFS(ctx, sqlDB, files)
	if err != nil {
		return SchemaStatus{}, err
	}
	if st.State, err = ReadState(ctx, sqlDB); err != nil {
		return SchemaStatus{}, err
	}
	st.BinaryMajor = Majors[len(Majors)-1].Major
	return st, nil
}

// MigrateFS is Migrate over any migrations directory.
func MigrateFS(ctx context.Context, sqlDB *sql.DB, files fs.FS) (int, error) {
	// A lock wait is bounded by ctx alone: goose's own default would give up after five minutes.
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, math.MaxInt32))
	if err != nil {
		return 0, err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files, goose.WithSessionLocker(locker))
	if err != nil {
		return 0, err
	}
	results, err := provider.Up(ctx)
	return len(results), err
}

// StatusFS is Status over any migrations directory.
func StatusFS(ctx context.Context, sqlDB *sql.DB, files fs.FS) (SchemaStatus, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, files)
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
