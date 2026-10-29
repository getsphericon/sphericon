package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

// exitNewerRelease is the exit status of `upgrade --check` when a newer release exists
// (upgrade.ErrNewerRelease, ADR 0034).
const exitNewerRelease = 10

// upgradeCommand is `sphericon upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart]`: it
// installs a release over the running binary, downloaded from GitHub or, with --from, already on
// the machine (ADR 0034). --check only asks whether a newer release exists.
func upgradeCommand() *cli.Command {
	return &cli.Command{
		Name:  "upgrade",
		Usage: "install a release over this binary",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "from", Usage: "directory with a release archive and checksums.txt (default: download from GitHub)"},
			&cli.StringFlag{Name: "version", Usage: "release to install (default: the latest stable one)"},
			&cli.BoolFlag{Name: "yes", Usage: "go past the Upgrade notes"},
			&cli.BoolFlag{Name: "check", Usage: "exit 0 when current, 10 when a newer release exists, 1 on error; change nothing"},
			&cli.BoolFlag{Name: "restart", Usage: "migrate in a transient unit, then restart the systemd service"},
			&cli.StringFlag{Name: "service", Value: "sphericon", Usage: "systemd unit that --restart restarts"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (%s)", cmd.Args().First(), upgrade.Usage)
			}
			return upgrade.Run(ctx, upgrade.Request{
				From:    cmd.String("from"),
				Version: cmd.String("version"),
				Yes:     cmd.Bool("yes"),
				Check:   cmd.Bool("check"),
				Restart: cmd.Bool("restart"),
				Service: cmd.String("service"),
				Out:     out(cmd),
			})
		},
	}
}
