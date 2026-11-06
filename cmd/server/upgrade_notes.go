package main

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/domain/instance/upgradenote"
)

// upgradeNotesCommand is `sphericon upgrade-notes --since <version>`: it prints the `Upgrade:`
// entries of every release after --since, from the changelog text it is given (the binary's
// embedded one), and nothing when there are none.
func upgradeNotesCommand(text string) *cli.Command {
	return &cli.Command{
		Name:  "upgrade-notes",
		Usage: "print the Upgrade notes of every release after a version",
		Flags: []cli.Flag{&cli.StringFlag{Name: "since", Usage: "the version you run now", Required: true}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			releases, err := upgradenote.Since(upgradenote.Parse(text), cmd.String("since"))
			if err != nil {
				return err
			}
			_, err = io.WriteString(out(cmd), upgradenote.Render(releases))
			return err
		},
	}
}
