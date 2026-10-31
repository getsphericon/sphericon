// Command sphericon is the one binary: the server, and the commands an operator runs against an
// Instance (migrate, doctor, upgrade, user, workspace, staff, preset and the others).
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/changelog"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/doctor"
)

// commandOps is everything the staff member commands need from the app. The app
// implements it (internal/app).
type commandOps interface {
	workspaceOps
	userOps
	staffOps
	serviceCredentialOps
	rekeyOps
	eventsOps
}

// deps is what the commands reach outside themselves for.
type deps struct {
	env string
	// changelog is the text `upgrade-notes` reads: the one embedded in this binary.
	changelog string
	now       func() time.Time
	// encryptionKey is the keyset the process is configured with (SPHERICON_ENCRYPTION_KEY or its
	// file), for the key commands that need no database.
	encryptionKey func() (string, error)
	// doctor is what `doctor` reaches outside the process for.
	doctor doctor.Seams
	// open boots the minimal command app (no listener, no event router, no workers) and
	// returns its ops and the function that shuts it down. It runs only once the
	// arguments have parsed.
	open func(ctx context.Context) (commandOps, func(), error)
}

func newDeps(env string) deps {
	return deps{
		env:           env,
		changelog:     changelog.Embedded(),
		now:           time.Now,
		encryptionKey: func() (string, error) { return config.EncryptionKey(env) },
		doctor:        doctor.DefaultSeams(),
		open: func(ctx context.Context) (commandOps, func(), error) {
			a, err := app.NewCommandApp(env)
			if err != nil {
				return nil, nil, err
			}
			return a, func() { cleanup.Shutdown(a.Shutdown(ctx)) }, nil
		},
	}
}

// newRoot builds the `sphericon` command tree. Without a subcommand it serves.
func newRoot(d deps) *cli.Command {
	cli.VersionPrinter = printVersion
	return &cli.Command{
		Name:    "sphericon",
		Usage:   "marketing automation platform",
		Version: buildinfo.Current().Version,
		Action:  serve(d.env),
		Commands: []*cli.Command{
			versionCommand(),
			genkeyCommand(),
			encryptionKeyCommand(d),
			migrateCommand(d.env),
			doctorCommand(d),
			upgradeCommand(),
			upgradeNotesCommand(d.changelog),
			workspaceCommand(d),
			userCommand(d),
			staffCommand(d),
			serviceCredentialCommand(d),
			eventsCommand(d),
			presetCommand(),
			healthcheckCommand(d),
		},
	}
}

// out is where a command writes its result.
func out(cmd *cli.Command) io.Writer { return cmd.Root().Writer }

// withOps runs a staff member command against the booted command app, under a timeout.
func (d deps) withOps(run func(ctx context.Context, cmd *cli.Command, ops commandOps) error) cli.ActionFunc {
	return d.withOpsWithin(30*time.Second, run)
}

// withOpsWithin is withOps with its own timeout, for a command that walks a whole table.
func (d deps) withOpsWithin(timeout time.Duration, run func(ctx context.Context, cmd *cli.Command, ops commandOps) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ops, closeApp, err := d.open(ctx)
		if err != nil {
			return err
		}
		defer closeApp()
		return run(ctx, cmd, ops)
	}
}

// group is a command that only holds subcommands: on its own, or with a name it does
// not know, it fails instead of printing help and exiting 0.
func group(name, usage string, subs ...*cli.Command) *cli.Command {
	return &cli.Command{
		Name:     name,
		Usage:    usage,
		Commands: subs,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown %s command %q", name, cmd.Args().First())
			}
			return fmt.Errorf("%s needs a command (see `sphericon %s --help`)", name, name)
		},
	}
}
