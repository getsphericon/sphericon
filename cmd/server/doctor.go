package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/doctor"
)

// doctorCommand is `sphericon doctor [--json]`: it checks the Instance's configuration the way
// the server reads it and prints one line per check with a fix for each failure (exit 1 when
// any failed, doctor.ErrChecksFailed).
func doctorCommand(d deps) *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "check this Instance's configuration and say how to fix what is wrong",
		Flags: []cli.Flag{&cli.BoolFlag{Name: "json", Usage: "print the report as JSON, for monitoring"}},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			return doctor.Diagnose(ctx, d.env, d.doctor, out(cmd), cmd.Bool("json"))
		},
	}
}
