package upgrade

import (
	"context"
	"errors"
	"iter"
	"os"

	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
)

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
}

// Upgrader installs a release over the running binary (ADR 0034): `sphericon upgrade`.
type Upgrader struct {
	// gh is where Run checks for a newer release and, without a directory, downloads it from.
	gh *GitHub
}

// New builds the upgrader over GitHub, the release source of Run; Step and Walk read the Source
// of their Options.
func New(gh *GitHub) *Upgrader { return &Upgrader{gh: gh} }

// Run does what r asks (ADR 0034), yielding its Progress. With Check it yields whether GitHub
// has a newer stable release (Latest) and takes no other option. Otherwise it walks the running
// binary to the release, from the directory From or from GitHub.
func (u *Upgrader) Run(ctx context.Context, r Request) iter.Seq2[Progress, error] {
	return progressOf(func(emit emitter) error { return u.run(ctx, r, emit) })
}

func (u *Upgrader) run(ctx context.Context, r Request, emit emitter) error {
	if r.Check {
		if r.From != "" || r.Version != "" || r.Yes || r.Restart {
			return errors.New(Usage)
		}
		latest, err := u.gh.CheckLatest(ctx, buildinfo.Current().Version)
		if err != nil {
			return err
		}
		return emit(latest)
	}
	var source Source = u.gh
	if r.From != "" {
		source = DirSource(r.From)
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
		Service: r.Service,
	}
	if r.Restart {
		sd := NewSystemd(r.Service)
		opts.Run, opts.PostSwap = sd.Run, sd.PostSwap
	}
	return walk(ctx, opts, emit)
}

// CheckLatest says whether GitHub has a stable release newer than current, changing nothing.
func (g *GitHub) CheckLatest(ctx context.Context, current string) (Latest, error) {
	latest, err := g.LatestRelease(ctx)
	if err != nil {
		return Latest{}, err
	}
	return Latest{
		Release: latest,
		Running: current,
		Newer:   semver.Compare(buildinfo.Semver(latest), buildinfo.Semver(current)) > 0,
	}, nil
}
