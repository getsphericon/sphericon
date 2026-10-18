package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	appdb "github.com/getsphericon/sphericon/internal/db"
)

// runMigrate is `sphericon migrate`: it applies the pending migrations with no time limit,
// or with --status only prints what would run.
func runMigrate(env string, args []string, out io.Writer) error {
	status := false
	for _, a := range args {
		if a != "--status" {
			return fmt.Errorf("unknown argument %q (usage: migrate [--status])", a)
		}
		status = true
	}
	cfg, err := config.Load(env)
	if err != nil {
		return err
	}
	if status {
		return printMigrationStatus(context.Background(), cfg, out)
	}
	applied, err := applyMigrations(context.Background(), cfg)
	if err != nil {
		return err
	}
	slog.Info("migrations applied", "count", applied)
	return nil
}

// migrateOnStart is the start with SPHERICON_DB_AUTO_MIGRATE on: the wait for the lock and the
// run take at most SPHERICON_DB_MIGRATE_TIMEOUT (0 = no limit).
func migrateOnStart(cfg *config.Config) error {
	ctx := context.Background()
	if cfg.MigrateTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.MigrateTimeout)
		defer cancel()
	}
	applied, err := applyMigrations(ctx, cfg)
	if err != nil {
		return fmt.Errorf("migrate within %s: %w", cfg.MigrateTimeout, err)
	}
	slog.Info("migrations applied", "count", applied)
	return nil
}

// applyMigrations opens a short-lived connection (separate from the app's DI pool), runs the
// guard (a migration is allowed, so only a binary that cannot run on the schema is refused) and
// applies the embedded migrations under the database lock; ctx is the only time limit.
func applyMigrations(ctx context.Context, cfg *config.Config) (int, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return 0, err
	}
	defer func() { _ = db.Close() }()
	if err := (appdb.Guard{Build: buildinfo.Current(), AutoMigrate: true}).Check(ctx, db); err != nil {
		return 0, err
	}
	return appdb.Migrate(ctx, db)
}

// guardStart is the start without auto-migrate: the process refuses, naming what to do, when
// it cannot run on the database as it is (ADR 0033).
func guardStart(ctx context.Context, cfg *config.Config) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return appdb.Guard{Build: buildinfo.Current()}.Check(ctx, db)
}

// printMigrationStatus prints the schema's state, the newest the binary knows and the
// pending migrations, and changes nothing.
func printMigrationStatus(ctx context.Context, cfg *config.Config, out io.Writer) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	st, err := appdb.Status(ctx, db)
	if err != nil {
		return err
	}
	writeMigrationStatus(out, st)
	return nil
}

func writeMigrationStatus(out io.Writer, st appdb.SchemaStatus) {
	_, _ = fmt.Fprintf(out, "schema:  %s\n", versionLabel(st.Current))
	_, _ = fmt.Fprintf(out, "binary:  %s\n", versionLabel(st.Newest))
	_, _ = fmt.Fprintf(out, "state:   Major %d, oldest release that runs: %s (this binary: Major %d)\n",
		st.State.Major, st.State.Floor, st.BinaryMajor)
	if len(st.Pending) == 0 {
		_, _ = fmt.Fprintln(out, "pending: none")
		return
	}
	_, _ = fmt.Fprintf(out, "pending: %d\n", len(st.Pending))
	for _, p := range st.Pending {
		_, _ = fmt.Fprintf(out, "  %s\n", p)
	}
}

func versionLabel(v int64) string {
	if v == 0 {
		return "empty"
	}
	return fmt.Sprintf("%d", v)
}
