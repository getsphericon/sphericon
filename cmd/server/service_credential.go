package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	serviceapi "github.com/getsphericon/sphericon/gen/service"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/ee/staff"
	"github.com/getsphericon/sphericon/ent"
)

// serviceCredentialOps is what the `service-credential` commands need from the app. The
// app implements it (internal/app).
type serviceCredentialOps interface {
	IssueServiceCredential(ctx context.Context, in staff.IssueInput) (*staff.Issued, error)
	RotateServiceCredential(ctx context.Context, id int64, expiresAt *time.Time) (*staff.Issued, error)
	RevokeServiceCredential(ctx context.Context, id int64) error
	ServiceCredentials(ctx context.Context, name string) ([]*ent.ServiceCredential, error)
}

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
				return staff.ReportIssued(out(cmd), issued, err)
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
				return staff.ReportRotated(out(cmd), id, issued, err)
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
				err = ops.RevokeServiceCredential(ctx, id)
				return staff.ReportRevoked(out(cmd), id, err)
			}),
		},
		&cli.Command{
			Name:      "list",
			Usage:     "list the credentials, optionally of one name",
			ArgsUsage: "[<name>]",
			Arguments: []cli.Argument{&cli.StringArg{Name: "name"}},
			Action: d.withOps(func(ctx context.Context, cmd *cli.Command, ops commandOps) error {
				list, err := ops.ServiceCredentials(ctx, cmd.StringArg("name"))
				return staff.ReportCredentials(out(cmd), list, d.now(), err)
			}),
		},
	)
}

func parseScopes(list string) []serviceapi.ServiceScope {
	var scopes []serviceapi.ServiceScope
	for part := range strings.SplitSeq(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			scopes = append(scopes, serviceapi.ServiceScope(part))
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
