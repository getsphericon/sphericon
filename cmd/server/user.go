package main

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/db"
)

// userOps is what the `user` staff member commands need from the app. The app
// implements it (internal/app); tests use a fake.
type userOps interface {
	ResetSecondFactor(ctx context.Context, email string) (bool, error)
	CreateOwner(ctx context.Context, email, name string) (password string, err error)
}

// userCommand is the `user` group. `create-owner <email> <name...>` (ADR 0042) is the
// explicit step that makes the first Owner of an Instance that closed sign-up.
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
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				email := cmd.StringArg("email")
				name := strings.TrimSpace(strings.Join(cmd.StringArgs("name"), " "))
				if name == "" {
					return errors.New("a name is required")
				}
				if _, err := mail.ParseAddress(email); err != nil {
					return fmt.Errorf("%q is not an email address", email)
				}
				password, err := ops.CreateOwner(ctx, email, name)
				if db.IsUniqueViolation(err) {
					return fmt.Errorf("a user with email %q already exists; nothing changed", email)
				}
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(out(cmd), "owner %q created with its first workspace \"Personal\"\npassword (shown once): %s\nsign in and change it\n", email, password)
				return nil
			}),
		},
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
