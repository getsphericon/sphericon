package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

// userOps is what the `user` staff member commands need from the app. The app
// implements it (internal/app); tests use a fake.
type userOps interface {
	ResetSecondFactor(ctx context.Context, email string) (bool, error)
}

// userCommand is `sphericon user reset-second-factor <email>` (ADR 0020): the way
// back in for a User locked out of their Second factor with nobody to reset it in
// the product (the sole Owner of a self-hosted instance).
func userCommand(d deps) *cli.Command {
	return group("user", "recover a User's access",
		&cli.Command{
			Name:      "reset-second-factor",
			Usage:     "clear a User's Second factor and recovery codes and end their sessions",
			ArgsUsage: "<email>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "email", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				email := cmd.StringArg("email")
				changed, err := ops.ResetSecondFactor(ctx, email)
				if err != nil {
					return err
				}
				if !changed {
					_, _ = fmt.Fprintf(out(cmd), "user %q has no second factor; nothing changed\n", email)
					return nil
				}
				_, _ = fmt.Fprintf(out(cmd), "second factor of %q reset: the factor and recovery codes are cleared and every session has ended\n", email)
				return nil
			}),
		},
	)
}
