package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/telemetry"
)

// stopTimeout bounds the shutdown of the app and the flush of the telemetry.
const stopTimeout = 10 * time.Second

// Serve is `sphericon` without a subcommand: it runs the server of env until ctx ends or the
// server stops by itself. An error means the server could not start.
func Serve(ctx context.Context, env string) error {
	cfg, err := config.Load(env)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	// Install the configured logger process-wide; river, watermill, and every
	// request-scoped handler emit through it from here on.
	logging.Setup(cfg)
	build := buildinfo.Current()
	slog.Info("starting sphericon", "version", build.Version, "commit", build.Commit)

	// Install the global OTel providers (traces + metrics). The ogen servers and
	// job/event instrumentation pick these up from the globals. The shutdown is
	// deferred at the top so it runs after the HTTP server has stopped and the app has
	// shut down (LIFO) — the last batched spans/metrics flush then.
	telShutdown, err := telemetry.Setup(context.WithoutCancel(ctx), cfg, env, telemetry.BuildInfo{Version: build.Version, Commit: build.Commit})
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
		defer cancel()
		if err := telShutdown(shutdownCtx); err != nil {
			slog.Error("telemetry shutdown", "err", err)
		}
	}()

	if err := db.PrepareStart(ctx, cfg); err != nil {
		return err
	}

	application, err := newApp(cfg, options{})
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}

	// Start binds the opt-in metrics listener, starts the event router and the job queue and
	// serves HTTP; any of them failing is fatal (a degraded instance that drops Broadcasts or
	// runs unmonitored is worse than none), after Start has unwound what it began.
	if err := application.Start(ctx); err != nil {
		return fmt.Errorf("start app: %w", err)
	}

	// The document renderer's runtime takes longer to start than a request may run on a small
	// machine, so it starts now, in the background, and not on the first save of a Broadcast.
	if err := application.WarmDocuments(ctx); err != nil {
		return fmt.Errorf("warm the document renderer: %w", err)
	}

	select {
	case <-ctx.Done():
	case err := <-application.Done():
		if err != nil {
			slog.Error("server stopped", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()
	if err := application.Close(shutdownCtx); err != nil {
		slog.Error("shutdown incomplete", "err", err)
	}
	return nil
}
