package commandline

import (
	"fmt"
	"io"
)

// StandingChange is what `sphericon workspace suspend` or `unsuspend` says of the Workspace it
// names: what the change did, or that the Workspace already stood so.
type StandingChange struct {
	changed, unchanged string
}

var (
	// Suspension is `sphericon workspace suspend`: outbound sending is frozen and the Owners told.
	Suspension = StandingChange{
		changed:   "suspended: outbound sending is frozen and the owner has been notified",
		unchanged: "is already suspended",
	}
	// Unsuspension is `sphericon workspace unsuspend`: the held sends resume.
	Unsuspension = StandingChange{
		changed:   "unsuspended: held sends will resume",
		unchanged: "is not suspended",
	}
)

// Report reports the change of the Workspace id: what it did when changed, that nothing changed
// otherwise, and nothing but the error of a change that failed.
func (c StandingChange) Report(w io.Writer, id string, changed bool, err error) error {
	if err != nil {
		return err
	}
	if !changed {
		_, _ = fmt.Fprintf(w, "workspace %q %s; nothing changed\n", id, c.unchanged)
		return nil
	}
	_, _ = fmt.Fprintf(w, "workspace %q %s\n", id, c.changed)
	return nil
}
