package secondfactor

import (
	"fmt"
	"io"
)

// ReportReset reports `sphericon user reset-second-factor` of the User with email: whether
// their Second factor was cleared, or they had none.
func ReportReset(w io.Writer, email string, changed bool, err error) error {
	if err != nil {
		return err
	}
	if !changed {
		_, _ = fmt.Fprintf(w, "user %q has no second factor; nothing changed\n", email)
		return nil
	}
	_, _ = fmt.Fprintf(w, "second factor of %q reset: the factor and recovery codes are cleared and every session has ended\n", email)
	return nil
}
