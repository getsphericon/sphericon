package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	sphericon "github.com/mokevnin/sphericon"
	"github.com/mokevnin/sphericon/config"
	"github.com/mokevnin/sphericon/internal/app"
	appdb "github.com/mokevnin/sphericon/internal/db"
	"github.com/mokevnin/sphericon/internal/jobs"
	"github.com/mokevnin/sphericon/internal/logging"
	"github.com/mokevnin/sphericon/internal/secrets"
	"github.com/mokevnin/sphericon/internal/telemetry"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Build metadata, injected via -ldflags by the Makefile / GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		slog.Info("sphericon version", "version", version, "commit", commit, "built", date)
		return
	}

	// `server genkey` prints a fresh ENCRYPTION_KEY, so a binary or image install
	// can bootstrap without the Go toolchain.
	if len(os.Args) > 1 && os.Args[1] == "genkey" {
		key, err := secrets.GenerateKeysetBase64()
		if err != nil {
			fatal("genkey", err)
		}
		fmt.Println(key)
		return
	}

	// `server migrate` applies pending migrations and exits — handy for a
	// separate orchestration step (init container, release job, manual run).
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrate(env); err != nil {
			fatal("migrate", err)
		}
		return
	}

	// `server workspace suspend|unsuspend <slug> …` is the operator toggle for a
	// Workspace's outbound-sending freeze (ADR 0007).
	if len(os.Args) > 1 && os.Args[1] == "workspace" {
		if err := runWorkspaceCommand(env, os.Args[2:]); err != nil {
			fatal("workspace", err)
		}
		return
	}

	// `server user reset-second-factor <email>` resets a User's Second factor for an
	// instance with nobody to do it in the product (ADR 0020).
	if len(os.Args) > 1 && os.Args[1] == "user" {
		if err := runUserCommand(env, os.Args[2:]); err != nil {
			fatal("user", err)
		}
		return
	}

	// `server operator create <email>` makes a platform Operator (ADR 0026): the only way
	// one comes to exist, since the /operator surface has no signup.
	if len(os.Args) > 1 && os.Args[1] == "operator" {
		if err := runOperatorCommand(env, os.Args[2:]); err != nil {
			fatal("operator", err)
		}
		return
	}

	cfg, err := config.Load(env)
	if err != nil {
		fatal("load config", err)
	}
	// Install the configured logger process-wide; river, watermill, and every
	// request-scoped handler emit through it from here on.
	logging.Setup(cfg)
	slog.Info("starting sphericon", "version", version, "commit", commit)

	// Install the global OTel providers (traces + metrics). The ogen servers and
	// job/event instrumentation pick these up from the globals. The shutdown is
	// deferred at the top level so it runs after the HTTP server has stopped and
	// the app has shut down (LIFO) — the last batched spans/metrics flush then.
	telShutdown, err := telemetry.Setup(context.Background(), cfg, env, telemetry.BuildInfo{Version: version, Commit: commit})
	if err != nil {
		fatal("init telemetry", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := telShutdown(shutdownCtx); err != nil {
			slog.Error("telemetry shutdown", "err", err)
		}
	}()

	// Opt-in self-migration on startup, so the single binary/image can come up
	// against a fresh database with no extra step.
	if cfg.AutoMigrate {
		if err := applyMigrations(cfg); err != nil {
			fatal("auto-migrate", err)
		}
		slog.Info("migrations applied")
	}

	application, err := app.New(env)
	if err != nil {
		fatal("init app", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start binds the opt-in metrics listener, starts the event router and the job
	// queue and serves HTTP; any of them failing is fatal (a degraded instance that
	// drops Broadcasts or runs unmonitored is worse than none), after Start has
	// unwound what it began.
	if err := application.Start(ctx); err != nil {
		fatal("start app", err)
	}

	select {
	case <-ctx.Done():
	case err := <-application.Done():
		if err != nil {
			slog.Error("server stopped", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := application.Close(shutdownCtx); err != nil {
		slog.Error("shutdown incomplete", "err", err)
	}
}

// fatal logs an error and exits non-zero (slog has no Fatal helper).
func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}

func runMigrate(env string) error {
	cfg, err := config.Load(env)
	if err != nil {
		return err
	}
	if err := applyMigrations(cfg); err != nil {
		return err
	}
	slog.Info("migrations applied")
	return nil
}

// applyMigrations opens a short-lived connection (separate from the app's DI
// pool) and applies the embedded migrations with goose.
func applyMigrations(cfg *config.Config) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	files, err := fs.Sub(sphericon.MigrationsFS, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err = provider.Up(ctx); err != nil {
		return err
	}

	// river owns its own schema (river_job, ...), which goose does not carry.
	pool, err := appdb.NewPGXPool(ctx, cfg.DatabaseURL, cfg.DBPool)
	if err != nil {
		return err
	}
	defer pool.Close()
	return jobs.Migrate(ctx, pool)
}

// runWorkspaceCommand boots the minimal operator app (no listener, no event router,
// no workers) and runs one workspace command against it.
func runWorkspaceCommand(env string, args []string) error {
	a, err := app.NewOperator(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runWorkspace(ctx, a, args, os.Stdout)
}

// runUserCommand boots the minimal operator app and runs one user command against it.
func runUserCommand(env string, args []string) error {
	a, err := app.NewOperator(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runUser(ctx, a, args, os.Stdout)
}

// runOperatorCommand boots the minimal operator app and runs one operator command
// against it.
func runOperatorCommand(env string, args []string) error {
	a, err := app.NewOperator(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runOperator(ctx, a, args, os.Stdout)
}
