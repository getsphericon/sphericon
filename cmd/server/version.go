package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

// printVersion makes the root `--version` flag print the same line as `sphericon version`
// (ADR 0034), where urfave's default would print only the version.
func printVersion(cmd *cli.Command) {
	_, _ = fmt.Fprintln(cmd.Root().Writer, buildinfo.Current().Line())
}

// versionCommand prints the build on one line (ADR 0034); --json prints the same as an object.
func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "print the build version",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "print the build as a JSON object"}},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			info := buildinfo.Current()
			if cmd.Bool("json") {
				return json.NewEncoder(out(cmd)).Encode(info)
			}
			_, err := fmt.Fprintln(out(cmd), info.Line())
			return err
		},
	}
}
