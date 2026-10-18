package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/changelog"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/telemetry"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	env := os.Getenv("SPHERICON_ENV")
	if env == "" {
		env = "development"
	}

	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		args := os.Args[2:]
		if os.Args[1] == "--version" {
			args = nil
		}
		if err := runVersion(os.Stdout, args); err != nil {
			fatal("version", err)
		}
		return
	}

	// `server upgrade-notes --since <version>` prints the Upgrade notes embedded in this binary.
	if len(os.Args) > 1 && os.Args[1] == "upgrade-notes" {
		if err := runUpgradeNotes(os.Stdout, os.Args[2:], changelog.Embedded()); err != nil {
			fatal("upgrade-notes", err)
		}
		return
	}

	// `server upgrade --from <dir>` installs a downloaded release over this binary (ADR 0034).
	if len(os.Args) > 1 && os.Args[1] == "upgrade" {
		if err := runUpgrade(context.Background(), os.Args[2:], os.Stdout); err != nil {
			if errors.Is(err, errNewerRelease) {
				os.Exit(exitNewerRelease)
			}
			fatal("upgrade", err)
		}
		return
	}

	// `server genkey` prints a fresh SPHERICON_ENCRYPTION_KEY, so a binary or image install
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
		if err := runMigrate(env, os.Args[2:], os.Stdout); err != nil {
			fatal("migrate", err)
		}
		return
	}

	// `server workspace suspend|unsuspend <slug> …` is the staff member toggle for a
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

	// `server staff member create <email>` makes a platform Staff member (ADR 0026): the only way
	// one comes to exist, since the /staff surface has no signup.
	if len(os.Args) > 1 && os.Args[1] == "staff" {
		if err := runStaffCommand(env, os.Args[2:]); err != nil {
			fatal("staff", err)
		}
		return
	}

	// `server service-credential create|rotate|revoke|list …` manages the Service
	// credentials of the Staff surface (ADR 0026): no endpoint can mint or change one.
	if len(os.Args) > 1 && os.Args[1] == "service-credential" {
		if err := runServiceCredentialCommand(env, os.Args[2:]); err != nil {
			fatal("service-credential", err)
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
	build := buildinfo.Current()
	slog.Info("starting sphericon", "version", build.Version, "commit", build.Commit)

	// Install the global OTel providers (traces + metrics). The ogen servers and
	// job/event instrumentation pick these up from the globals. The shutdown is
	// deferred at the top level so it runs after the HTTP server has stopped and
	// the app has shut down (LIFO) — the last batched spans/metrics flush then.
	telShutdown, err := telemetry.Setup(context.Background(), cfg, env, telemetry.BuildInfo{Version: build.Version, Commit: build.Commit})
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
		if err := migrateOnStart(cfg); err != nil {
			fatal("auto-migrate", err)
		}
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

// runWorkspaceCommand boots the minimal staff member app (no listener, no event router,
// no workers) and runs one workspace command against it.
func runWorkspaceCommand(env string, args []string) error {
	a, err := app.NewCommandApp(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runWorkspace(ctx, a, args, os.Stdout)
}

// runUserCommand boots the minimal staff member app and runs one user command against it.
func runUserCommand(env string, args []string) error {
	a, err := app.NewCommandApp(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runUser(ctx, a, args, os.Stdout)
}

// runServiceCredentialCommand boots the minimal staff member app and runs one
// service-credential command against it.
func runServiceCredentialCommand(env string, args []string) error {
	a, err := app.NewCommandApp(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runServiceCredential(ctx, a, time.Now, args, os.Stdout)
}

// runStaffCommand boots the minimal staff member app and runs one staff member command
// against it.
func runStaffCommand(env string, args []string) error {
	a, err := app.NewCommandApp(env)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer func() { _ = a.Shutdown(ctx) }()
	return runStaff(ctx, a, args, os.Stdout)
}
