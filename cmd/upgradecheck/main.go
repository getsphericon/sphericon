// Command upgradecheck runs the Upgrade checks of ADR 0033 (internal/upgradecheck) over a pull
// request in CI.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/upgradecheck"
)

// The names of the flags the commands read back, each declared and looked up by its constant so a
// misspelt lookup fails to compile instead of reading a zero value.
const (
	flagTitle    = "title"
	flagBodyFile = "body-file"
	flagBase     = "base"
)

func main() {
	if err := newRoot().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRoot() *cli.Command {
	// A flag holds the value it parsed, so each command gets flags of its own.
	title := func() cli.Flag {
		return &cli.StringFlag{Name: flagTitle, Required: true, Usage: "pull request title (the squash commit subject)"}
	}
	bodyFile := func() cli.Flag { return &cli.StringFlag{Name: flagBodyFile, Usage: "file with the pull request body"} }
	base := func() cli.Flag {
		return &cli.StringFlag{Name: flagBase, Value: "origin/main", Usage: "ref the pull request is compared against"}
	}
	return &cli.Command{
		Name:  "upgradecheck",
		Usage: "the Upgrade checks of ADR 0033 over a pull request",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unknown command %q (see `upgradecheck --help`)", cmd.Args().First())
			}
			return errors.New("a command is required (see `upgradecheck --help`)")
		},
		Commands: []*cli.Command{
			{
				Name:  "breaking",
				Usage: "a breaking change needs a breaking title and an `Upgrade:` line in its footer",
				Flags: []cli.Flag{title(), bodyFile(), base()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					body, err := readBody(cmd.String(flagBodyFile))
					if err != nil {
						return err
					}
					return upgradecheck.Breaking(ctx, cmd.String(flagTitle), body, cmd.String(flagBase))
				},
			},
			{
				Name:  "migrations",
				Usage: "lint the migrations the pull request adds with Squawk",
				Flags: []cli.Flag{title(), base()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					linted, err := upgradecheck.Migrations(ctx, cmd.String(flagTitle), cmd.String(flagBase))
					if err != nil {
						return err
					}
					if len(linted) == 0 {
						fmt.Println("no migration added; nothing to lint")
					}
					return nil
				},
			},
			{
				Name:  "immutable",
				Usage: "a migration already on the base branch is never edited, renamed or deleted",
				Flags: []cli.Flag{bodyFile(), base()},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					body, err := readBody(cmd.String(flagBodyFile))
					if err != nil {
						return err
					}
					return upgradecheck.Immutable(ctx, body, cmd.String(flagBase))
				},
			},
		},
	}
}

// readBody reads the pull request body from the file the flag names, or none when it names none.
func readBody(bodyFile string) (string, error) {
	if bodyFile == "" {
		return "", nil
	}
	b, err := os.ReadFile(bodyFile)
	return string(b), err
}
