package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

// ErrNewerRelease is what a check returns when a newer release exists; the command exits 10 on
// it (ADR 0034).
var ErrNewerRelease = errors.New("a newer release is available")

// Usage is how `sphericon upgrade` is called.
const Usage = "usage: upgrade [--from <dir>] [--version vX.Y.Z] [--yes] [--restart] | upgrade --check"

// Request is what `sphericon upgrade` was asked to do.
type Request struct {
	// From is a directory with a release archive and checksums.txt; empty downloads from GitHub.
	From string
	// Version pins the release to install; empty is the latest stable one.
	Version string
	// Yes goes past the Upgrade notes.
	Yes bool
	// Check only asks whether a newer release exists and changes nothing.
	Check bool
	// Restart migrates in a transient unit, then restarts the systemd unit Service.
	Restart bool
	Service string
	Out     io.Writer
}

// Run does what r asks (ADR 0034). With Check it says whether GitHub has a newer stable release
// (ErrNewerRelease) and takes no other option. Otherwise it walks the running binary to the
// release, from the directory From or from GitHub.
func Run(ctx context.Context, r Request) error {
	if r.Check {
		if r.From != "" || r.Version != "" || r.Yes || r.Restart {
			return errors.New(Usage)
		}
		gh, err := NewGitHub()
		if err != nil {
			return err
		}
		return CheckLatest(ctx, gh, buildinfo.Current().Version, r.Out)
	}
	source, err := r.source()
	if err != nil {
		return err
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	opts := Options{
		Source:  source,
		Target:  target,
		Current: buildinfo.Current().Version,
		Version: r.Version,
		Yes:     r.Yes,
		Out:     r.Out,
		Service: r.Service,
	}
	if r.Restart {
		sd := Systemd{Service: r.Service, Out: r.Out}
		opts.Run, opts.PostSwap = sd.Run, sd.PostSwap
	}
	_, err = Walk(ctx, opts)
	return err
}

func (r Request) source() (Source, error) {
	if r.From != "" {
		return DirSource(r.From), nil
	}
	return NewGitHub()
}

// CheckLatest says on w whether gh has a stable release newer than current, changing nothing:
// ErrNewerRelease when it has.
func CheckLatest(ctx context.Context, gh *GitHub, current string, w io.Writer) error {
	latest, err := gh.LatestRelease(ctx)
	if err != nil {
		return err
	}
	if semver.Compare(buildinfo.Semver(latest), buildinfo.Semver(current)) > 0 {
		_, err := fmt.Fprintf(w, "A newer release is available: %s (running %s). Run `sphericon upgrade`.\n", latest, current)
		return errors.Join(err, ErrNewerRelease)
	}
	_, err = fmt.Fprintf(w, "Up to date: %s is the latest release.\n", latest)
	return err
}
