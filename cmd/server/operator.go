package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mokevnin/sphericon/ee/operator"
)

// operatorOps is what the `operator` commands need from the app. The app implements
// it (internal/app); tests use a fake.
type operatorOps interface {
	CreateOperator(ctx context.Context, email string) (password string, err error)
	ResetOperatorTOTP(ctx context.Context, email string) error
}

const operatorUsage = "usage: sphericon operator create|reset-totp <email>"

// runOperator executes the `operator` commands (ADR 0026). `create <email>` is the only
// way a platform Operator comes to exist, since there is no signup; it prints the
// one-time password, and the Operator enrols a TOTP at first login. `reset-totp <email>`
// is the only way a lost TOTP is reset (no web flow can): it clears the factor and ends
// the Operator's sessions, and the Operator re-enrols at next login.
func runOperator(ctx context.Context, ops operatorOps, args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New(operatorUsage)
	}
	email := args[1]
	switch args[0] {
	case "create":
		password, err := ops.CreateOperator(ctx, email)
		switch {
		case errors.Is(err, operator.ErrDuplicate):
			return fmt.Errorf("an operator with email %q already exists; nothing changed", email)
		case errors.Is(err, operator.ErrNotLicensed):
			return errors.New("this instance has no operator license (feature `operator`)")
		case err != nil:
			return err
		}
		_, _ = fmt.Fprintf(out, "operator %q created\npassword (shown once): %s\nthe operator enrols a TOTP at first login\n", email, password)
	case "reset-totp":
		switch err := ops.ResetOperatorTOTP(ctx, email); {
		case errors.Is(err, operator.ErrNotFound):
			return fmt.Errorf("no operator with email %q; nothing changed", email)
		case errors.Is(err, operator.ErrNotLicensed):
			return errors.New("this instance has no operator license (feature `operator`)")
		case err != nil:
			return err
		}
		_, _ = fmt.Fprintf(out, "totp of operator %q reset: every session has ended and the operator must re-enrol at next login\n", email)
	default:
		return fmt.Errorf("unknown operator command %q\n%s", args[0], operatorUsage)
	}
	return nil
}
