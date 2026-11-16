package main

import (
	"context"
	"net/http"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/healthcheck"
)

// healthcheckCommand is `sphericon healthcheck`: the image's HEALTHCHECK. It exits non-zero
// unless the server on PORT answers its /healthz with 200.
func healthcheckCommand() *cli.Command {
	return &cli.Command{
		Name:  "healthcheck",
		Usage: "exit 0 when the server on PORT answers /healthz, 1 otherwise",
		Action: noArgs(func(ctx context.Context, _ *cli.Command) error {
			port, err := config.Port()
			if err != nil {
				return err
			}
			return healthcheck.Probe(ctx, http.DefaultClient, port)
		}),
	}
}
