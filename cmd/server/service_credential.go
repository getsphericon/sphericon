package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/internal/commandline"
)

// serviceCredentialCommand is `sphericon service-credential create|rotate|revoke|list` (ADR 0026),
// the only way a Service credential is created, rotated or revoked: no endpoint can. A value is
// shown once, when it is created, and only a hash of it is stored. A service may hold two
// active credentials at once, so a rotation is `rotate` (a second credential with the
// same name and scopes) followed by `revoke` of the old one once the service has switched.
func serviceCredentialCommand(d deps) *cli.Command {
	expiresIn := func() cli.Flag {
		return &cli.DurationFlag{
			Name:  flagExpiresIn,
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
		in := cmd.Duration(flagExpiresIn)
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
			Description: "scopes: usage:read, workspaces:read, users:read, domains:read, holds:write",
			Flags:       []cli.Flag{expiresIn()},
			Arguments: []cli.Argument{
				&cli.StringArg{Name: flagName, Required: true},
				&cli.StringArg{Name: flagScopes, Required: true},
			},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				credentials, err := c.ServiceCredentials()
				if err != nil {
					return err
				}
				issued, err := credentials.IssueServiceCredential(ctx, servicecredentials.IssueInput{
					Name:      cmd.StringArg(flagName),
					Scopes:    parseScopes(cmd.StringArg(flagScopes)),
					ExpiresAt: expiry(cmd),
				})
				return commandline.ReportCredentialIssued(out(cmd), issued, err)
			}),
		},
		&cli.Command{
			Name:      "rotate",
			Usage:     "issue a second credential with the same name and scopes",
			ArgsUsage: "<id>",
			Flags:     []cli.Flag{expiresIn()},
			Arguments: []cli.Argument{&cli.IntArg{Name: flagID, Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				id, err := credentialID(cmd)
				if err != nil {
					return err
				}
				credentials, err := c.ServiceCredentials()
				if err != nil {
					return err
				}
				issued, err := credentials.RotateServiceCredential(ctx, id, expiry(cmd))
				return commandline.ReportCredentialRotated(out(cmd), id, issued, err)
			}),
		},
		&cli.Command{
			Name:      "revoke",
			Usage:     "refuse a credential from now on",
			ArgsUsage: "<id>",
			Arguments: []cli.Argument{&cli.IntArg{Name: flagID, Required: true}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				id, err := credentialID(cmd)
				if err != nil {
					return err
				}
				credentials, err := c.ServiceCredentials()
				if err != nil {
					return err
				}
				err = credentials.RevokeServiceCredential(ctx, id)
				return commandline.ReportCredentialRevoked(out(cmd), id, err)
			}),
		},
		&cli.Command{
			Name:      "list",
			Usage:     "list the credentials, optionally of one name",
			ArgsUsage: "[<name>]",
			Arguments: []cli.Argument{&cli.StringArg{Name: flagName}},
			Action: d.withCore(func(ctx context.Context, cmd *cli.Command, c core) error {
				credentials, err := c.ServiceCredentials()
				if err != nil {
					return err
				}
				list, err := credentials.ServiceCredentials(ctx, cmd.StringArg(flagName))
				return commandline.ReportCredentials(out(cmd), list, d.now(), err)
			}),
		},
	)
}

func parseScopes(list string) []servicecredentials.Scope {
	var scopes []servicecredentials.Scope
	for part := range strings.SplitSeq(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			scopes = append(scopes, servicecredentials.Scope(part))
		}
	}
	return scopes
}

// credentialID is the `<id>` argument of rotate and revoke.
func credentialID(cmd *cli.Command) (int64, error) {
	id := int64(cmd.IntArg(flagID))
	if id < 1 {
		return 0, fmt.Errorf("%d is not a service credential id (see `list`)", id)
	}
	return id, nil
}
