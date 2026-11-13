// Command sphericon is the one binary: the server, and the commands an operator runs against an
// Instance (migrate, doctor, upgrade, user, workspace, staff, preset and the others). The commands
// are a driver (ADR 0111): each reads its arguments, calls one operation of a domain module or a
// use case and prints the result. The program they call is built by the wiring (internal/app).
package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/doctor"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/instance/upgradenote"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/usecase/signup"
)

// core is what the staff member commands call: the domain modules and use cases of the program
// the command app builds, each built only when a command asks for it. The app implements it
// (internal/app).
type core interface {
	Suspender() (*suspension.Suspender, error)
	SecondFactors() (*secondfactors.Module, error)
	SignUp() (*signup.UseCase, error)
	StaffMembers() (*staffmembers.Module, error)
	ServiceCredentials() (*servicecredentials.Module, error)
	SecretReencryption() (app.SecretReencryption, error)
	Workspaces() (*workspaces.Module, error)
	Bus() (*events.Bus, error)
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
	// open boots the minimal command app (no listener, no workers) and
	// returns its core and the function that shuts it down. It runs only once the
	// arguments have parsed.
	open func(ctx context.Context) (core, func(), error)
}

func newDeps(env string) deps {
	return deps{
		env:           env,
		changelog:     upgradenote.Embedded(),
		now:           time.Now,
		encryptionKey: func() (string, error) { return config.EncryptionKey(env) },
		doctor:        doctor.DefaultSeams(),
		open: func(ctx context.Context) (core, func(), error) {
			a, err := app.NewCommandApp(env)
			if err != nil {
				return nil, nil, err
			}
			return a, func() {
				report := a.Shutdown(ctx)
				cleanup.Shutdown(report)
			}, nil
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
			presetCommand(),
			eventsCommand(d),
			healthcheckCommand(d),
		},
	}
}

// out is where a command writes its result.
func out(cmd *cli.Command) io.Writer { return cmd.Root().Writer }

// withCore runs a staff member command against the booted command app, under a timeout.
func (d deps) withCore(run func(ctx context.Context, cmd *cli.Command, c core) error) cli.ActionFunc {
	return d.withCoreWithin(30*time.Second, run)
}

// withCoreWithin is withCore with its own timeout, for a command that walks a whole table.
func (d deps) withCoreWithin(timeout time.Duration, run func(ctx context.Context, cmd *cli.Command, c core) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		c, closeApp, err := d.open(ctx)
		if err != nil {
			return err
		}
		defer closeApp()
		return run(ctx, cmd, c)
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
