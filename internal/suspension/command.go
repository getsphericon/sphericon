package suspension

import (
	"fmt"
	"io"
)

// ReportSuspended reports `sphericon workspace suspend` of the Workspace id: whether its
// outbound sending is now frozen, or was already.
func ReportSuspended(w io.Writer, id string, changed bool, err error) error {
	if err != nil {
		return err
	}
	if !changed {
		_, _ = fmt.Fprintf(w, "workspace %q is already suspended; nothing changed\n", id)
		return nil
	}
	_, _ = fmt.Fprintf(w, "workspace %q suspended: outbound sending is frozen and the owner has been notified\n", id)
	return nil
}

// ReportUnsuspended reports `sphericon workspace unsuspend` of the Workspace id: whether its
// held sends resume, or it was not suspended.
func ReportUnsuspended(w io.Writer, id string, changed bool, err error) error {
	if err != nil {
		return err
	}
	if !changed {
		_, _ = fmt.Fprintf(w, "workspace %q is not suspended; nothing changed\n", id)
		return nil
	}
	_, _ = fmt.Fprintf(w, "workspace %q unsuspended: held sends will resume\n", id)
	return nil
}
