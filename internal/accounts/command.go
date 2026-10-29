package accounts

import (
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/db"
)

// ReportOwnerCreated reports `sphericon user create-owner` (ADR 0042): the Owner, its first
// Workspace and the one-time password, or that the email is taken and nothing changed.
func ReportOwnerCreated(w io.Writer, email, password string, err error) error {
	switch {
	case db.IsUniqueViolation(err):
		return fmt.Errorf("a user with email %q already exists; nothing changed", email)
	case err != nil:
		return err
	}
	_, _ = fmt.Fprintf(w, "owner %q created with its first workspace %q\npassword (shown once): %s\nsign in and change it\n", email, FirstWorkspaceName, password)
	return nil
}
