package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/doctor"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/upgrade"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	env := os.Getenv("SPHERICON_ENV")
	if env == "" {
		env = "development"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRoot(newDeps(env)).Run(ctx, os.Args)
	stop()
	if err != nil {
		if errors.Is(err, upgrade.ErrNewerRelease) {
			// `upgrade --check` found a newer release: it already said so on stdout (ADR 0034).
			os.Exit(exitNewerRelease)
		}
		if errors.Is(err, doctor.ErrChecksFailed) {
			// `doctor` already printed what failed.
			os.Exit(1)
		}
		fatal("sphericon", err)
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

// serve runs the server until it is interrupted or stops by itself.
func serve(env string) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unknown command %q (see `sphericon --help`)", cmd.Args().First())
		}
		return app.Serve(ctx, env)
	}
}

// fatal logs an error and exits non-zero (slog has no Fatal helper).
func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
