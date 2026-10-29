package main

import (
	"context"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/ee/staff"
)

// staffOps is what the `staff` commands need from the app. The app implements
// it (internal/app).
type staffOps interface {
	CreateStaffMember(ctx context.Context, email string) (password string, err error)
	ResetStaffTOTP(ctx context.Context, email string) error
}

// staffCommand is `sphericon staff create|reset-totp <email>` (ADR 0026). `create` is the only
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
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				email := cmd.StringArg("email")
				password, err := ops.CreateStaffMember(ctx, email)
				return staff.ReportCreated(out(cmd), email, password, err)
			}),
		},
		&cli.Command{
			Name:      "reset-totp",
			Usage:     "clear a Staff member's TOTP and end their sessions",
			ArgsUsage: "<email>",
			Arguments: []cli.Argument{&cli.StringArg{Name: "email", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				email := cmd.StringArg("email")
				err := ops.ResetStaffTOTP(ctx, email)
				return staff.ReportTOTPReset(out(cmd), email, err)
			}),
		},
	)
}
