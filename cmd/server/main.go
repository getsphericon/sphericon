package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/app"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/telemetry"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Build metadata, injected via -ldflags by the Makefile / GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	env := os.Getenv("SPHERICON_ENV")
	if env == "" {
		env = "development"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := newRoot(newDeps(env)).Run(ctx, os.Args); err != nil {
		stop()
		fatal("sphericon", err)
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print the build version",
		Action: func(_ context.Context, cmd *cli.Command) error {
			_, _ = fmt.Fprintf(out(cmd), "sphericon version %s (commit %s, built %s)\n", version, commit, date)
			return nil
		},
	}
}

// genkeyCommand prints a fresh SPHERICON_ENCRYPTION_KEY, so a binary or image install
// can bootstrap without the Go toolchain. Its stdout is the key and nothing else: scripts
// capture it.
func genkeyCommand() *cli.Command {
	return &cli.Command{
		Name:  "genkey",
		Usage: "print a fresh SPHERICON_ENCRYPTION_KEY",
		Action: func(_ context.Context, cmd *cli.Command) error {
			key, err := secrets.GenerateKeysetBase64()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out(cmd), key)
			return nil
		},
	}
}

// migrateCommand applies pending migrations and exits: handy for a separate
// orchestration step (init container, release job, manual run).
func migrateCommand(env string) *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "apply pending migrations and exit",
		Action: func(ctx context.Context, _ *cli.Command) error {
			return runMigrate(ctx, env)
		},
	}
}

// serve runs the server until it is interrupted or stops by itself.
func serve(env string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unknown command %q (see `sphericon --help`)", cmd.Args().First())
		}
		runServer(ctx, env)
		return nil
	}
}

func runServer(ctx context.Context, env string) {
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
	telShutdown, err := telemetry.Setup(context.WithoutCancel(ctx), cfg, env, telemetry.BuildInfo{Version: version, Commit: commit})
	if err != nil {
		fatal("init telemetry", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := telShutdown(shutdownCtx); err != nil {
			slog.Error("telemetry shutdown", "err", err)
		}
	}()

	// Opt-in self-migration on startup, so the single binary/image can come up
	// against a fresh database with no extra step.
	if cfg.AutoMigrate {
		if err := applyMigrations(ctx, cfg); err != nil {
			fatal("auto-migrate", err)
		}
		slog.Info("migrations applied")
	}

	application, err := app.New(env)
	if err != nil {
		fatal("init app", err)
	}

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

func runMigrate(ctx context.Context, env string) error {
	cfg, err := config.Load(env)
	if err != nil {
		return err
	}
	if err := applyMigrations(ctx, cfg); err != nil {
		return err
	}
	slog.Info("migrations applied")
	return nil
}

// applyMigrations opens a short-lived connection (separate from the app's DI
// pool) and applies the embedded migrations with goose.
func applyMigrations(ctx context.Context, cfg *config.Config) error {
	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return appdb.Migrate(ctx, db)
}
