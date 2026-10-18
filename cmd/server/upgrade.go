package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// errNewerRelease is what `upgrade --check` returns when a newer release exists; main turns it
// into exit status 10 (ADR 0034).
var errNewerRelease = errors.New("a newer release is available")

// exitNewerRelease is the exit status of `upgrade --check` when a newer release exists.
const exitNewerRelease = 10

// runUpgrade is `sphericon upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--check]`: it
// installs a release over the running binary, downloaded from GitHub or, with --from, already on
// the machine (ADR 0034). --check only asks whether a newer release exists.
func runUpgrade(ctx context.Context, args []string, out io.Writer) error {
	const usage = "usage: upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart] | upgrade --check"
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "directory with a release archive and checksums.txt (default: download from GitHub)")
	version := fs.String("version", "", "release to install (default: the latest stable one)")
	yes := fs.Bool("yes", false, "go past the Upgrade notes")
	check := fs.Bool("check", false, "exit 0 when current, 10 when a newer release exists, 1 on error; change nothing")
	restart := fs.Bool("restart", false, "migrate in a transient unit, then restart the systemd service")
	service := fs.String("service", "sphericon", "systemd unit that --restart restarts")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w (%s)", err, usage)
	}
	if *check {
		if *from != "" || *version != "" || *yes || *restart {
			return errors.New(usage)
		}
		gh, err := upgrade.NewGitHub()
		if err != nil {
			return err
		}
		return checkRelease(ctx, gh, buildinfo.Current().Version, out)
	}
	var source upgrade.Source
	if *from != "" {
		source = upgrade.DirSource(*from)
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
		Version: *version,
		Yes:     *yes,
		Out:     out,
	}
	if *restart {
		sd := upgrade.Systemd{Service: *service, Out: out}
		opts.Run, opts.PostSwap = sd.Run, sd.PostSwap
	}
	_, err = upgrade.Step(ctx, opts)
	return err
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
