package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/getsphericon/sphericon/internal/buildinfo"
	"github.com/getsphericon/sphericon/internal/upgrade"
)

// runUpgrade is `sphericon upgrade --from <dir> [--version vX.Y.Z] [--yes]`: it installs a
// release already on the machine over the running binary (ADR 0034).
func runUpgrade(ctx context.Context, args []string, out io.Writer) error {
	const usage = "usage: upgrade --from <dir> [--version vX.Y.Z] [--yes]"
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	from := fs.String("from", "", "directory with a release archive and checksums.txt")
	version := fs.String("version", "", "release to install (default: the newest in --from)")
	yes := fs.Bool("yes", false, "go past the Upgrade notes")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w (%s)", err, usage)
	}
	if *from == "" {
		return errors.New(usage)
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = upgrade.Step(ctx, upgrade.Options{
		Source:  upgrade.DirSource(*from),
		Target:  target,
		Current: buildinfo.Current().Version,
		Version: *version,
		Yes:     *yes,
		Out:     out,
	})
	return err
}
