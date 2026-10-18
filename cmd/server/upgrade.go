package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// errNewerRelease is what `upgrade --check` returns when a newer release exists; main turns it
// into exit status 10 (ADR 0034).
var errNewerRelease = errors.New("a newer release is available")

// exitNewerRelease is the exit status of `upgrade --check` when a newer release exists.
const exitNewerRelease = 10

// upgradeCommand is `sphericon upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart]`: it
// installs a release over the running binary, downloaded from GitHub or, with --from, already on
// the machine (ADR 0034). --check only asks whether a newer release exists.
func upgradeCommand() *cli.Command {
	const usage = "usage: upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart] | upgrade --check"
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
				return fmt.Errorf("unexpected argument %q (%s)", cmd.Args().First(), usage)
			}
			from, version := cmd.String("from"), cmd.String("version")
			if cmd.Bool("check") {
				if from != "" || version != "" || cmd.Bool("yes") || cmd.Bool("restart") {
					return errors.New(usage)
				}
				gh, err := upgrade.NewGitHub()
				if err != nil {
					return err
				}
				return checkRelease(ctx, gh, buildinfo.Current().Version, out(cmd))
			}
			var source upgrade.Source
			if from != "" {
				source = upgrade.DirSource(from)
			} else {
				gh, err := upgrade.NewGitHub()
				if err != nil {
					return err
				}
				source = gh
			}
			target, err := os.Executable()
			if err != nil {
				return err
			}
			opts := upgrade.Options{
				Source:  source,
				Target:  target,
				Current: buildinfo.Current().Version,
				Version: version,
				Yes:     cmd.Bool("yes"),
				Out:     out(cmd),
			}
			if cmd.Bool("restart") {
				sd := upgrade.Systemd{Service: cmd.String("service"), Out: out(cmd)}
				opts.Run, opts.PostSwap = sd.Run, sd.PostSwap
			}
			_, err = upgrade.Walk(ctx, opts)
			return err
		},
	}
}

// checkRelease says whether GitHub has a stable release newer than current, changing nothing.
func checkRelease(ctx context.Context, gh *upgrade.GitHub, current string, out io.Writer) error {
	latest, err := gh.LatestRelease(ctx)
	if err != nil {
		return err
	}
	if semver.Compare("v"+latest, "v"+strings.TrimPrefix(current, "v")) > 0 {
		_, err := fmt.Fprintf(out, "A newer release is available: %s (running %s). Run `sphericon upgrade`.\n", latest, current)
		return errors.Join(err, errNewerRelease)
	}
	_, err = fmt.Fprintf(out, "Up to date: %s is the latest release.\n", latest)
	return err
}
