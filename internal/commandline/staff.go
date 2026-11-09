package commandline

import (
	"errors"
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
)

// What the `sphericon staff` commands print (ADR 0026, ADR 0063): each Report function takes
// what the module returned, prints the outcome to w, or says in its error why nothing changed.

// ReportStaffCreated reports `staff create`: the one-time password, shown once.
func ReportStaffCreated(w io.Writer, email, password string, err error) error {
	switch {
	case errors.Is(err, staffmembers.ErrDuplicate):
		return fmt.Errorf("a staff with email %q already exists; nothing changed", email)
	case errors.Is(err, staffmembers.ErrSignInOff):
		return errors.New("staff sign-in is off: set SPHERICON_STAFF_JWT_SECRET (e.g. `openssl rand -hex 32`, distinct from SPHERICON_JWT_SECRET) and restart; nothing changed")
	case err != nil:
		return err
	}
	_, _ = fmt.Fprintf(w, "staff %q created\npassword (shown once): %s\nthe staff enrols a TOTP at first login\n", email, password)
	return nil
}

// ReportTOTPReset reports `staff reset-totp`.
func ReportTOTPReset(w io.Writer, email string, err error) error {
	switch {
	case errors.Is(err, staffmembers.ErrNotFound):
		return fmt.Errorf("no staff with email %q; nothing changed", email)
	case err != nil:
		return err
	}
	_, _ = fmt.Fprintf(w, "totp of staff %q reset: every session has ended and the staff must re-enrol at next login\n", email)
	return nil
}
