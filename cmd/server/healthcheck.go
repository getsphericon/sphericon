package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/config"
)

// healthcheckCommand is `sphericon healthcheck`: the image's HEALTHCHECK. The runtime image is
// scratch, with no shell and no wget, so the binary probes its own /healthz on PORT and exits
// non-zero unless it answers 200.
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

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			url := "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			if err != nil {
				return err
			}

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("healthcheck: %w", err)
			}
			defer func() { _ = res.Body.Close() }()

			_, _ = io.Copy(io.Discard, res.Body)
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("healthcheck: GET %s answered %s", url, res.Status)
			}
			return nil
		},
	}
}
