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

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signup"
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// core is what the staff member commands call: the domain modules and use cases of the program
// the command app builds, each built only when a command asks for it. The app implements it
// (internal/app).
type core interface {
	Suspender() (*suspension.Suspender, error)
	SecondFactors() (*secondfactors.Module, error)
	SignUp() (*signup.Module, error)
	StaffMembers() (*staffmembers.Module, error)
	ServiceCredentials() (*servicecredentials.Module, error)
	SecretReencryption() (app.SecretReencryption, error)
	Workspaces() (*workspaces.Module, error)
	Bus() (*events.Bus, error)
}

// newRoot builds the `sphericon` command tree for the environment env. Without a subcommand it
// serves.
func newRoot(env string) *cli.Command {
	cli.VersionPrinter = printVersion
	return &cli.Command{
		Name:    "sphericon",
		Usage:   "marketing automation platform",
		Version: buildinfo.Current().Version,
		Action:  serve(env),
		Commands: []*cli.Command{
			versionCommand(),
			genkeyCommand(),
			encryptionKeyCommand(env),
			migrateCommand(env),
			doctorCommand(env),
			upgradeCommand(),
			upgradeNotesCommand(),
			workspaceCommand(env),
			userCommand(env),
			staffCommand(env),
			serviceCredentialCommand(env),
			presetCommand(),
			eventsCommand(env),
			healthcheckCommand(),
		},
	}
}

// now is the clock the commands read: they run outside the container that provides app.Clock, so
// it is the wall clock.
func now() time.Time { return time.Now() }

// out is where a command writes its result.
func out(cmd *cli.Command) io.Writer { return cmd.Root().Writer }

// withCore runs a staff member command against the command app of env (no listener, no
// workers), booted once the arguments have parsed, under a timeout.
func withCore(env string, run func(ctx context.Context, cmd *cli.Command, c core) error) cli.ActionFunc {
	return withCoreWithin(env, 30*time.Second, run)
}

// withCoreWithin is withCore with its own timeout, for a command that walks a whole table.
func withCoreWithin(env string, timeout time.Duration, run func(ctx context.Context, cmd *cli.Command, c core) error) cli.ActionFunc {
	return noArgs(func(ctx context.Context, cmd *cli.Command) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		a, err := app.NewCommandApp(env)
		if err != nil {
			return err
		}
		defer func() { cleanup.Shutdown(a.Shutdown(ctx)) }()
		return run(ctx, cmd, a)
	})
}

// noArgs is action for a command that takes no positional argument: given one, it fails instead
// of ignoring it.
func noArgs(action cli.ActionFunc) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		if cmd.Args().Present() {
			return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
		}
		return action(ctx, cmd)
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
