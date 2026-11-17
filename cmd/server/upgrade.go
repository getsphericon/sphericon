package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/getsphericon/sphericon/internal/commandline"
	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
)

// exitNewerRelease is the exit status of `upgrade --check` when a newer release exists
// (commandline.ErrNewerRelease, ADR 0034).
const exitNewerRelease = 10

// upgradeCommand is `sphericon upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart]`: it
// installs a release over the running binary, downloaded from GitHub or, with --from, already on
// the machine (ADR 0034). --check only asks whether a newer release exists.
func upgradeCommand() *cli.Command {
	return &cli.Command{
		Name:  "upgrade",
		Usage: "install a release over this binary",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flagFrom, Usage: "directory with a release archive and checksums.txt (default: download from GitHub)"},
			&cli.StringFlag{Name: flagVersion, Usage: "release to install (default: the latest stable one)"},
			&cli.BoolFlag{Name: flagYes, Usage: "go past the Upgrade notes"},
			&cli.BoolFlag{Name: flagCheck, Usage: "exit 0 when current, 10 when a newer release exists, 1 on error; change nothing"},
			&cli.BoolFlag{Name: flagRestart, Usage: "migrate in a transient unit, then restart the systemd service"},
			&cli.StringFlag{Name: flagService, Value: "sphericon", Usage: "systemd unit that --restart restarts"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Present() {
				return fmt.Errorf("unexpected argument %q (%s)", cmd.Args().First(), upgrade.Usage)
			}
			from, restart, service := cmd.String(flagFrom), cmd.Bool(flagRestart), cmd.String(flagService)
			// A check changes nothing, so it takes no option that chooses how an install runs.
			if cmd.Bool(flagCheck) && (from != "" || restart) {
				return errors.New(upgrade.Usage)
			}
			endpoint, err := upgrade.NewGitHubEndpoint("")
			if err != nil {
				return err
			}
			// The seams of this Upgrade: the release comes from GitHub or, with --from, from a
			// directory; --restart migrates in a transient unit and restarts the systemd service.
			var seams upgrade.Seams
			if from != "" {
				seams.Source = upgrade.DirSource(from)
			}
			if restart {
				sd := upgrade.NewSystemd(service)
				seams.Run, seams.PostSwap = sd.Run, sd.PostSwap
			}
			progress := upgrade.New(upgrade.NewGitHub(endpoint), seams).Run(ctx, upgrade.Request{
				Version: cmd.String(flagVersion),
				Yes:     cmd.Bool(flagYes),
				Check:   cmd.Bool(flagCheck),
				Service: service,
			})
			return commandline.WriteUpgrade(out(cmd), progress)
		},
	}
}
