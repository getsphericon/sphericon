package staff

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// What the `sphericon service-credential` commands print (ADR 0026): each
// Report function takes what the module returned, prints the outcome to w, or says in its error
// why nothing changed.

// errNoLicense explains ErrNotLicensed to the operator.
var errNoLicense = errors.New("this instance has no staff license (feature `staff`)")

// ReportIssued reports `service-credential create`: the credential and its value, shown once.
func ReportIssued(w io.Writer, c *Issued, err error) error {
	if err != nil {
		return explainCredential(err, 0)
	}
	writeIssued(w, c)
	return nil
}

// ReportRotated reports `service-credential rotate` of the credential id: the new credential,
// and that id stays active until it is revoked.
func ReportRotated(w io.Writer, id int64, c *Issued, err error) error {
	if err != nil {
		return explainCredential(err, id)
	}
	writeIssued(w, c)
	_, _ = fmt.Fprintf(w, "credential %d stays active until you revoke it: `sphericon service-credential revoke %d`\n", id, id)
	return nil
}

// ReportRevoked reports `service-credential revoke` of the credential id.
func ReportRevoked(w io.Writer, id int64, err error) error {
	if err != nil {
		return explainCredential(err, id)
	}
	_, _ = fmt.Fprintf(w, "service credential %d revoked: it is refused from now on\n", id)
	return nil
}

// ReportCredentials reports `service-credential list`: one row per credential with its state
// at now (active, revoked or expired), scopes, expiry and when it was last used.
func ReportCredentials(w io.Writer, list []*ent.ServiceCredential, now time.Time, err error) error {
	if err != nil {
		return explainCredential(err, 0)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tSTATE\tSCOPES\tEXPIRES\tLAST USED")
	for _, c := range list {
		_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n",
			c.ID, c.Name, credentialState(c, now), strings.Join(c.Scopes, ","), formatTime(c.ExpiresAt), formatTime(c.LastUsedAt))
	}
	_ = tw.Flush()
	return nil
}

func writeIssued(w io.Writer, c *Issued) {
	_, _ = fmt.Fprintf(w, "service credential %d created for %q\nscopes: %s\nexpires: %s\nvalue (shown once, only a hash is stored): %s\n",
		c.ID, c.Name, joinScopes(c.Scopes), formatTime(c.ExpiresAt), c.Value)
}

func credentialState(c *ent.ServiceCredential, now time.Time) string {
	switch {
	case c.RevokedAt != nil:
		return "revoked"
	case !active(c, now):
		return "expired"
	}
	return "active"
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func joinScopes(scopes []serviceapi.ServiceScope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}

// explainCredential says why a command on the credential id changed nothing.
func explainCredential(err error, id int64) error {
	switch {
	case errors.Is(err, ErrNotLicensed):
		return errNoLicense
	case errors.Is(err, ErrInvalidScope):
		return errors.New("a service credential needs one or more known scopes (usage:read, workspaces:read, users:read, domains:read, holds:write)")
	case errors.Is(err, ErrInvalidCredential):
		return errors.New("a service credential needs a name and an expiry in the future")
	case errors.Is(err, ErrCredentialNotFound):
		return fmt.Errorf("no service credential %d; nothing changed", id)
	case errors.Is(err, ErrCredentialInactive):
		return fmt.Errorf("service credential %d is revoked or expired; nothing changed", id)
	}
	return err
}
