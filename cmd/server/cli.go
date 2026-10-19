package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/changelog"
)

// commandOps is everything the staff member commands need from the app. The app
// implements it (internal/app); tests use fakes.
type commandOps interface {
	workspaceOps
	userOps
	staffOps
	serviceCredentialOps
}

// deps is what the commands reach outside themselves for, so tests can swap it.
type deps struct {
	env string
	// changelog is the text `upgrade-notes` reads: the one embedded in this binary.
	changelog string
	now       func() time.Time
	// doctor is what `doctor` reaches outside the process for.
	doctor doctorSeams
	// open boots the minimal command app (no listener, no event router, no workers) and
	// returns its ops and the function that shuts it down. It runs only once the
	// arguments have parsed.
	open func(ctx context.Context) (commandOps, func(), error)
}

func newDeps(env string) deps {
	return deps{
		env:       env,
		changelog: changelog.Embedded(),
		now:       time.Now,
		doctor:    newDoctorSeams(),
		open: func(ctx context.Context) (commandOps, func(), error) {
			a, err := app.NewCommandApp(env)
			if err != nil {
				return nil, nil, err
			}
			return a, func() { _ = a.Shutdown(ctx) }, nil
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
			migrateCommand(d.env),
			doctorCommand(d),
			upgradeCommand(),
			upgradeNotesCommand(d.changelog),
			workspaceCommand(d),
			userCommand(d),
			staffCommand(d),
			serviceCredentialCommand(d),
			presetCommand(),
		},
	}
}

// out is where a command writes its result.
func out(cmd *cli.Command) io.Writer { return cmd.Root().Writer }

// withOps runs a staff member command against the booted command app, under a timeout.
func (d deps) withOps(run func(ctx context.Context, cmd *cli.Command, ops commandOps) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
