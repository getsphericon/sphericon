package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/ee/staff"
)

// staffOps is what the `staff` commands need from the app. The app implements
// it (internal/app); tests use a fake.
type staffOps interface {
	CreateStaffMember(ctx context.Context, email string) (password string, err error)
	ResetStaffTOTP(ctx context.Context, email string) error
}

const staffUsage = "usage: sphericon staff create|reset-totp <email>"

// runStaff executes the `staff` commands (ADR 0026). `create <email>` is the only
// way a platform Staff member comes to exist, since there is no signup; it prints the
// one-time password, and the Staff member enrols a TOTP at first login. `reset-totp <email>`
// is the only way a lost TOTP is reset (no web flow can): it clears the factor and ends
// the Staff member's sessions, and the Staff member re-enrols at next login.
func runStaff(ctx context.Context, ops staffOps, args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New(staffUsage)
	}
	email := args[1]
	switch args[0] {
	case "create":
		password, err := ops.CreateStaffMember(ctx, email)
		switch {
		case errors.Is(err, staff.ErrDuplicate):
			return fmt.Errorf("a staff with email %q already exists; nothing changed", email)
		case errors.Is(err, staff.ErrNotLicensed):
			return errors.New("this instance has no staff license (feature `staff`)")
		case err != nil:
			return err
		}
		_, _ = fmt.Fprintf(out, "staff %q created\npassword (shown once): %s\nthe staff enrols a TOTP at first login\n", email, password)
	case "reset-totp":
		switch err := ops.ResetStaffTOTP(ctx, email); {
		case errors.Is(err, staff.ErrNotFound):
			return fmt.Errorf("no staff with email %q; nothing changed", email)
		case errors.Is(err, staff.ErrNotLicensed):
			return errors.New("this instance has no staff license (feature `staff`)")
		case err != nil:
			return err
		}
		_, _ = fmt.Fprintf(out, "totp of staff %q reset: every session has ended and the staff must re-enrol at next login\n", email)
	default:
		return fmt.Errorf("unknown staff command %q\n%s", args[0], staffUsage)
	}
	return nil
}
