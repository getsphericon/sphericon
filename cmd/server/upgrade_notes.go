package main

import (
	"context"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/domain/instance/upgradenote"
)

// upgradeNotesCommand is `sphericon upgrade-notes --since <version>`: it prints the `Upgrade:`
// entries of every release after --since, from the changelog embedded in this binary, and
// nothing when there are none.
func upgradeNotesCommand() *cli.Command {
	return &cli.Command{
		Name:  "upgrade-notes",
		Usage: "print the Upgrade notes of every release after a version",
		Flags: []cli.Flag{&cli.StringFlag{Name: flagSince, Usage: "the version you run now", Required: true}},
		Action: noArgs(func(_ context.Context, cmd *cli.Command) error {
			releases, err := upgradenote.Since(upgradenote.Parse(upgradenote.Embedded()), cmd.String(flagSince))
			if err != nil {
				return err
			}
			_, err = io.WriteString(out(cmd), upgradenote.Render(releases))
			return err
		}),
	}
}
