package main

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/doctor"
)

// doctorCommand is `sphericon doctor [--json]`: it checks the Instance's configuration the way
// the server reads it and prints one line per check with a fix for each failure (exit 1 when
// any failed, doctor.ErrChecksFailed).
func doctorCommand(env string) *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "check this Instance's configuration and say how to fix what is wrong",
		Flags: []cli.Flag{&cli.BoolFlag{Name: flagJSON, Usage: "print the report as JSON, for monitoring"}},
		Action: noArgs(func(ctx context.Context, cmd *cli.Command) error {
			return doctor.Diagnose(ctx, env, doctor.DefaultSeams(), out(cmd), cmd.Bool(flagJSON))
		}),
	}
}
