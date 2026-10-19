package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

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

const noLicense = "this instance has no staff license (feature `staff`)"

// serviceCredentialCommand is `sphericon service-credential create|rotate|revoke|list` (ADR 0026),
// the only way a Service credential is created, rotated or revoked: no endpoint can. A value is
// shown once, when it is created, and only a hash of it is stored. A service may hold two
// active credentials at once, so a rotation is `rotate` (a second credential with the
// same name and scopes) followed by `revoke` of the old one once the service has switched.
func serviceCredentialCommand(d deps) *cli.Command {
	expiresIn := func() cli.Flag {
		return &cli.DurationFlag{
			Name:  "expires-in",
			Usage: "expire the credential this long from now (default: never)",
			Validator: func(v time.Duration) error {
				if v < 0 {
					return errors.New("--expires-in must be a positive duration")
				}
				return nil
			},
		}
	}
	// expiry is the flag's deadline, nil when it was not given.
	expiry := func(cmd *cli.Command) *time.Time {
		in := cmd.Duration("expires-in")
		if in == 0 {
			return nil
		}
		at := d.now().Add(in)
		return &at
	}
	return group("service-credential", "manage the Service credentials of the Staff surface",
		&cli.Command{
			Name:        "create",
			Usage:       "create a Service credential and print its value once",
			ArgsUsage:   "<name> <scope>[,<scope>...]",
			Description: "scopes: usage:read, workspaces:read, domains:read, holds:write",
			Flags:       []cli.Flag{expiresIn()},
			Arguments: []cli.Argument{
				&cli.StringArg{Name: "name", Required: true},
				&cli.StringArg{Name: "scopes", Required: true},
			},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				issued, err := ops.IssueServiceCredential(ctx, staff.IssueInput{
					Name:      cmd.StringArg("name"),
					Scopes:    parseScopes(cmd.StringArg("scopes")),
					ExpiresAt: expiry(cmd),
				})
				if err != nil {
					return serviceCredentialError(err, 0)
				}
				showIssued(out(cmd), issued)
				return nil
			}),
		},
		&cli.Command{
			Name:      "rotate",
			Usage:     "issue a second credential with the same name and scopes",
			ArgsUsage: "<id>",
			Flags:     []cli.Flag{expiresIn()},
			Arguments: []cli.Argument{&cli.IntArg{Name: "id", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				id, err := credentialID(cmd)
				if err != nil {
					return err
				}
				issued, err := ops.RotateServiceCredential(ctx, id, expiry(cmd))
				if err != nil {
					return serviceCredentialError(err, id)
				}
				showIssued(out(cmd), issued)
				_, _ = fmt.Fprintf(out(cmd), "credential %d stays active until you revoke it: `sphericon service-credential revoke %d`\n", id, id)
				return nil
			}),
		},
		&cli.Command{
			Name:      "revoke",
			Usage:     "refuse a credential from now on",
			ArgsUsage: "<id>",
			Arguments: []cli.Argument{&cli.IntArg{Name: "id", Required: true}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				id, err := credentialID(cmd)
				if err != nil {
					return err
				}
				if err := ops.RevokeServiceCredential(ctx, id); err != nil {
					return serviceCredentialError(err, id)
				}
				_, _ = fmt.Fprintf(out(cmd), "service credential %d revoked: it is refused from now on\n", id)
				return nil
			}),
		},
		&cli.Command{
			Name:      "list",
			Usage:     "list the credentials, optionally of one name",
			ArgsUsage: "[<name>]",
			Arguments: []cli.Argument{&cli.StringArg{Name: "name"}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				list, err := ops.ServiceCredentials(ctx, cmd.StringArg("name"))
				if err != nil {
					return serviceCredentialError(err, 0)
				}
				listServiceCredentials(out(cmd), list, d.now())
				return nil
			}),
		},
	)
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

// credentialID is the `<id>` argument of rotate and revoke.
func credentialID(cmd *cli.Command) (int64, error) {
	id := int64(cmd.IntArg("id"))
	if id < 1 {
		return 0, fmt.Errorf("%d is not a service credential id (see `list`)", id)
	}
	return id, nil
}

func serviceCredentialError(err error, id int64) error {
	switch {
	case errors.Is(err, staff.ErrNotLicensed):
		return errors.New(noLicense)
	case errors.Is(err, staff.ErrInvalidScope):
		return fmt.Errorf("a service credential needs one or more known scopes (usage:read, workspaces:read, domains:read, holds:write)")
	case errors.Is(err, staff.ErrInvalidCredential):
		return errors.New("a service credential needs a name and an expiry in the future")
	case errors.Is(err, staff.ErrCredentialNotFound):
		return fmt.Errorf("no service credential %d; nothing changed", id)
	case errors.Is(err, staff.ErrCredentialInactive):
		return fmt.Errorf("service credential %d is revoked or expired; nothing changed", id)
	}
	return err
}
