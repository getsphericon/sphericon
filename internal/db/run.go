package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// guardTimeout bounds the guard at start and `migrate --status`, which only read.
const guardTimeout = 30 * time.Second

// ApplyMigrations is `sphericon migrate`: on a short-lived connection to url (separate from the
// app's pool) it runs the guard (a migration is allowed, so only a binary that cannot run on the
// schema is refused) and applies the embedded migrations under the database lock, reporting how
// many ran. ctx is the only time limit.
func ApplyMigrations(ctx context.Context, url string) (int, error) {
	return withDB(url, func(sqlDB *sql.DB) (int, error) {
		if err := (Guard{Build: buildinfo.Current(), AutoMigrate: true}).Check(ctx, sqlDB); err != nil {
			return 0, err
		}
		return Migrate(ctx, sqlDB)
	})
}

// PrepareStart readies the database before the server starts. With SPHERICON_DB_AUTO_MIGRATE on
// it applies the pending migrations, the wait for the lock and the run taking at most
// SPHERICON_DB_MIGRATE_TIMEOUT (0 = no limit), so the single binary can come up against a fresh
// database with no extra step. Otherwise the guard refuses, naming what to do, a database the
// binary cannot run on as it is (ADR 0033), except for the migrations a development build lets
// its error page run (config.MigratesFromThePage).
func PrepareStart(ctx context.Context, cfg *config.Config) error {
	if cfg.AutoMigrate {
		if err := migrateOnStart(ctx, cfg); err != nil {
			return fmt.Errorf("auto-migrate: %w", err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()
	_, err := withDB(cfg.DatabaseURL, func(sqlDB *sql.DB) (struct{}, error) {
		return struct{}{}, Guard{Build: buildinfo.Current(), AllowPending: cfg.MigratesFromThePage()}.Check(ctx, sqlDB)
	})
	if err != nil {
		return fmt.Errorf("schema guard: %w", err)
	}
	return nil
}

func migrateOnStart(ctx context.Context, cfg *config.Config) error {
	if cfg.MigrateTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.MigrateTimeout)
		defer cancel()
	}
	applied, err := ApplyMigrations(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("migrate within %s: %w", cfg.MigrateTimeout, err)
	}
	slog.Info("migrations applied", "count", applied)
	return nil
}

// WriteStatus is `sphericon migrate --status`: it prints the schema's state, the newest
// migration the binary knows and the pending ones, and changes nothing. asJSON prints instead
// what the Major walk of `upgrade` reads from the release it is about to install: the
// database's state and this binary's table of Majors (upgrade.Boundaries).
func WriteStatus(ctx context.Context, url string, w io.Writer, asJSON bool) error {
	ctx, cancel := context.WithTimeout(ctx, guardTimeout)
	defer cancel()
	st, err := withDB(url, func(sqlDB *sql.DB) (SchemaStatus, error) { return Status(ctx, sqlDB) })
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(w).Encode(boundaries(st))
	}
	writeStatusText(w, st)
	return nil
}

func boundaries(st SchemaStatus) upgrade.Boundaries {
	b := upgrade.Boundaries{State: upgrade.MajorFloor{Major: st.State.Major, Floor: st.State.Floor}}
	for _, m := range Majors {
		b.Majors = append(b.Majors, upgrade.MajorFloor{Major: m.Major, Floor: m.Floor})
	}
	return b
}

func writeStatusText(w io.Writer, st SchemaStatus) {
	_, _ = fmt.Fprintf(w, "schema:  %s\n", versionLabel(st.Current))
	_, _ = fmt.Fprintf(w, "binary:  %s\n", versionLabel(st.Newest))
	_, _ = fmt.Fprintf(w, "state:   Major %d, oldest release that runs: %s (this binary: Major %d)\n",
		st.State.Major, st.State.Floor, st.BinaryMajor)
	if len(st.Pending) == 0 {
		_, _ = fmt.Fprintln(w, "pending: none")
		return
	}
	_, _ = fmt.Fprintf(w, "pending: %d\n", len(st.Pending))
	for _, p := range st.Pending {
		_, _ = fmt.Fprintf(w, "  %s\n", p)
	}
}

func versionLabel(v int64) string {
	if v == 0 {
		return "empty"
	}
	return strconv.FormatInt(v, 10)
}

// withDB runs fn on a short-lived connection to url that it closes afterwards.
func withDB[T any](url string, fn func(sqlDB *sql.DB) (T, error)) (T, error) {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		var zero T
		return zero, err
	}
	defer cleanup.Close(sqlDB, "database")
	return fn(sqlDB)
}
