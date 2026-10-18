package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
)

// serviceCredentialOps is what the `service-credential` commands need from the app. The
// app implements it (internal/app); tests use a fake.
type serviceCredentialOps interface {
	IssueServiceCredential(ctx context.Context, in staff.IssueInput) (*staff.Issued, error)
	RotateServiceCredential(ctx context.Context, id int64, expiresAt *time.Time) (*staff.Issued, error)
	RevokeServiceCredential(ctx context.Context, id int64) error
	ServiceCredentials(ctx context.Context, name string) ([]*ent.ServiceCredential, error)
}

const serviceCredentialUsage = `usage: sphericon service-credential <command>
  create [--expires-in <duration>] <name> <scope>[,<scope>...]
  rotate [--expires-in <duration>] <id>
  revoke <id>
  list [<name>]
scopes: usage:read, workspaces:read, holds:write`

const noLicense = "this instance has no operator license (feature `operator`)"

// runServiceCredential executes the `service-credential` commands (ADR 0026), the only
// way a Service credential is created, rotated or revoked: no endpoint can. A value is
// shown once, when it is created, and only a hash of it is stored. A service may hold two
// active credentials at once, so a rotation is `rotate` (a second credential with the
// same name and scopes) followed by `revoke` of the old one once the service has switched.
func runServiceCredential(ctx context.Context, ops serviceCredentialOps, now func() time.Time, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(serviceCredentialUsage)
	}
	command, rest := args[0], args[1:]
	fs := flag.NewFlagSet("service-credential "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	expiresIn := fs.Duration("expires-in", 0, "")
	pos, err := parseInterleaved(fs, rest)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, serviceCredentialUsage)
	}
	var expiresAt *time.Time
	if *expiresIn < 0 {
		return errors.New("--expires-in must be a positive duration")
	}
	if *expiresIn > 0 {
		at := now().Add(*expiresIn)
		expiresAt = &at
	}
	switch command {
	case "create":
		if len(pos) != 2 {
			return errors.New(serviceCredentialUsage)
		}
		return createServiceCredential(ctx, ops, out, staff.IssueInput{Name: pos[0], Scopes: parseScopes(pos[1]), ExpiresAt: expiresAt})
	case "rotate":
		id, err := parseID(pos)
		if err != nil {
			return err
		}
		issued, err := ops.RotateServiceCredential(ctx, id, expiresAt)
		if err != nil {
			return serviceCredentialError(err, id)
		}
		showIssued(out, issued)
		_, _ = fmt.Fprintf(out, "credential %d stays active until you revoke it: `sphericon service-credential revoke %d`\n", id, id)
	case "revoke":
		id, err := parseID(pos)
		if err != nil {
			return err
		}
		if err := ops.RevokeServiceCredential(ctx, id); err != nil {
			return serviceCredentialError(err, id)
		}
		_, _ = fmt.Fprintf(out, "service credential %d revoked: it is refused from now on\n", id)
	case "list":
		if len(pos) > 1 {
			return errors.New(serviceCredentialUsage)
		}
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		list, err := ops.ServiceCredentials(ctx, name)
		if err != nil {
			return serviceCredentialError(err, 0)
		}
		listServiceCredentials(out, list, now())
	default:
		return fmt.Errorf("unknown service-credential command %q\n%s", command, serviceCredentialUsage)
	}
	return nil
}

func createServiceCredential(ctx context.Context, ops serviceCredentialOps, out io.Writer, in staff.IssueInput) error {
	issued, err := ops.IssueServiceCredential(ctx, in)
	if err != nil {
		return serviceCredentialError(err, 0)
	}
	showIssued(out, issued)
	return nil
}

func showIssued(out io.Writer, c *staff.Issued) {
	_, _ = fmt.Fprintf(out, "service credential %d created for %q\nscopes: %s\nexpires: %s\nvalue (shown once, only a hash is stored): %s\n",
		c.ID, c.Name, joinScopes(c.Scopes), formatTime(c.ExpiresAt, "never"), c.Value)
}

func listServiceCredentials(out io.Writer, list []*ent.ServiceCredential, now time.Time) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tSTATE\tSCOPES\tEXPIRES\tLAST USED")
	for _, c := range list {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n",
			c.ID, c.Name, credentialState(c, now), strings.Join(c.Scopes, ","), formatTime(c.ExpiresAt, "never"), formatTime(c.LastUsedAt, "never"))
	}
	_ = w.Flush()
}

func credentialState(c *ent.ServiceCredential, now time.Time) string {
	switch {
	case c.RevokedAt != nil:
		return "revoked"
	case c.ExpiresAt != nil && !now.Before(*c.ExpiresAt):
		return "expired"
	}
	return "active"
}

func formatTime(t *time.Time, none string) string {
	if t == nil {
		return none
	}
	return t.UTC().Format(time.RFC3339)
}

func joinScopes(scopes []staff.Scope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = string(s)
	}
	return strings.Join(parts, ",")
}

func parseScopes(list string) []staff.Scope {
	var scopes []staff.Scope
	for part := range strings.SplitSeq(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			scopes = append(scopes, staff.Scope(part))
		}
	}
	return scopes
}

func parseID(pos []string) (int64, error) {
	if len(pos) != 1 {
		return 0, errors.New(serviceCredentialUsage)
	}
	id, err := strconv.ParseInt(pos[0], 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%q is not a service credential id (see `list`)", pos[0])
	}
	return id, nil
}

// parseInterleaved parses flags wherever they stand among the positional arguments.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return pos, nil
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
}

func serviceCredentialError(err error, id int64) error {
	switch {
	case errors.Is(err, staff.ErrNotLicensed):
		return errors.New(noLicense)
	case errors.Is(err, staff.ErrInvalidScope):
		return fmt.Errorf("a service credential needs one or more known scopes (usage:read, workspaces:read, holds:write)")
	case errors.Is(err, staff.ErrInvalidCredential):
		return errors.New("a service credential needs a name and an expiry in the future")
	case errors.Is(err, staff.ErrCredentialNotFound):
		return fmt.Errorf("no service credential %d; nothing changed", id)
	case errors.Is(err, staff.ErrCredentialInactive):
		return fmt.Errorf("service credential %d is revoked or expired; nothing changed", id)
	}
	return err
}
