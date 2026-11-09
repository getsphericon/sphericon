package main

import (
	"context"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/urfave/cli/v3"
)

// staffCommand is `sphericon staff create|reset-totp <email>` (ADR 0026, ADR 0063), on every
// Instance without a License key. `create` is the only
// way a platform Staff member comes to exist, since there is no signup; it prints the
// one-time password, and the Staff member enrols a TOTP at first login. `reset-totp`
// is the only way a lost TOTP is reset (no web flow can): it clears the factor and ends
// the Staff member's sessions, and the Staff member re-enrols at next login.
func staffCommand(d deps) *cli.Command {
	return group("staff", "manage platform Staff members",
		&cli.Command{
			Name:      "create",
			Usage:     "create a Staff member and print their one-time password",
			ArgsUsage: "<email>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "email", Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				members, err := c.StaffMembers()
				if err != nil {
					return err
				}
				email := cmd.StringArg("email")
				password, err := members.Create(ctx, email)
				return commandline.ReportStaffCreated(out(cmd), email, password, err)
			}),
		},
		&cli.Command{
			Name:      "reset-totp",
			Usage:     "clear a Staff member's TOTP and end their sessions",
			ArgsUsage: "<email>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "email", Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				members, err := c.StaffMembers()
				if err != nil {
					return err
				}
				email := cmd.StringArg("email")
				err = members.ResetTOTP(ctx, email)
				return commandline.ReportTOTPReset(out(cmd), email, err)
			}),
		},
	)
}
