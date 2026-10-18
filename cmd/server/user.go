package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// userOps is what the `user` staff member commands need from the app. The app
// implements it (internal/app); tests use a fake.
type userOps interface {
	ResetSecondFactor(ctx context.Context, email string) (bool, error)
}

const userUsage = "usage: sphericon user reset-second-factor <email>"

// runUser executes `sphericon user reset-second-factor <email>` (ADR 0020): the way
// back in for a User locked out of their Second factor with nobody to reset it in
// the product (the sole Owner of a self-hosted instance).
func runUser(ctx context.Context, ops userOps, args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New(userUsage)
	}
	email := args[1]
	switch args[0] {
	case "reset-second-factor":
		changed, err := ops.ResetSecondFactor(ctx, email)
		if err != nil {
			return err
		}
		if !changed {
			_, _ = fmt.Fprintf(out, "user %q has no second factor; nothing changed\n", email)
			return nil
		}
		_, _ = fmt.Fprintf(out, "second factor of %q reset: the factor and recovery codes are cleared and every session has ended\n", email)
	default:
		return fmt.Errorf("unknown user command %q\n%s", args[0], userUsage)
	}
	return nil
}
