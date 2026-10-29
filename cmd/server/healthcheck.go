package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/healthcheck"
)

// healthcheckCommand is `sphericon healthcheck`: the image's HEALTHCHECK. It exits non-zero
// unless the server on PORT answers its /healthz with 200.
func healthcheckCommand(d deps) *cli.Command {
	return &cli.Command{
		Name:  "healthcheck",
		Usage: "exit 0 when the server on PORT answers /healthz, 1 otherwise",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (see `%s --help`)", cmd.Args().First(), cmd.FullName())
			}
			port, err := config.Port(d.env)
			if err != nil {
				return err
			}
			return healthcheck.Probe(ctx, http.DefaultClient, port)
		},
	}
}
