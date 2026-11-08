package main

import (
	"context"
	"errors"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
)

// userCommand is the `user` group. `create-owner <email> <name...>` (ADR 0042) makes an Owner
// from a shell, with no Sign-up.
// `reset-second-factor <email>` (ADR 0020) is the way back in for a User locked out of
// their Second factor with nobody to reset it in the product (the sole Owner of a
// self-hosted instance).
func userCommand(d deps) *cli.Command {
	return group("user", "create a first Owner or recover a User's access",
		&cli.Command{
			Name:      "create-owner",
			Usage:     "create a verified User with its first Workspace and print its one-time password",
			ArgsUsage: "<email> <name...>",
			Arguments: []cli.Argument{
				&cli.StringArg{Name: "email", Required: true},
				&cli.StringArgs{Name: "name", Min: 1, Max: -1},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				email := cmd.StringArg("email")
				name := strings.TrimSpace(strings.Join(cmd.StringArgs("name"), " "))
				if name == "" {
					return errors.New("a name is required")
				}
				signUp, err := c.SignUp()
				if err != nil {
					return err
				}
				password, err := signUp.CreateOwner(ctx, email, name)
				return users.ReportOwnerCreated(out(cmd), email, password, err)
			}),
		},
		&cli.Command{
			Name:      "reset-second-factor",
			Usage:     "clear a User's Second factor and recovery codes and end their sessions",
			ArgsUsage: "<email>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "email", Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				reset, err := c.SecondFactorReset()
				if err != nil {
					return err
				}
				email := cmd.StringArg("email")
				return commandline.ReportSecondFactorReset(out(cmd), email, reset.ResetByStaff(ctx, email, "cli"))
			}),
		},
	)
}
