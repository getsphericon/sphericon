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

// Request is what `sphericon upgrade` was asked to do. Where the release comes from (`--from`) and
// how it is migrated and restarted (`--restart`) are the Upgrader's Seams, which the command
// wires.
type Request struct {
	// Version pins the release to install; empty is the latest stable one.
	Version string
	// Yes goes past the Upgrade notes.
	Yes bool
	// Check only asks whether a newer release exists and changes nothing.
	Check bool
	// Service is the systemd unit the restart hint names.
	Service string
}

// Upgrader installs a release over the running binary (ADR 0034): `sphericon upgrade`.
type Upgrader struct {
	// gh is where Run checks for a newer release, and the Source when seams name none.
	gh    *GitHub
	seams Seams
}

// New builds the upgrader over GitHub, where Run checks for a newer release, and the seams every
// Step and Walk reach outside the process through; a nil seam is its default (Seams).
func New(gh *GitHub, seams Seams) *Upgrader {
	if seams.Source == nil && gh != nil {
		seams.Source = gh
	}
	if seams.Container == nil {
		seams.Container = DetectContainer
	}
	if seams.Run == nil {
		seams.Run = execRun
	}
	return &Upgrader{gh: gh, seams: seams}
}

// bind gives o the Upgrader's seams.
func (u *Upgrader) bind(o Options) Options {
	o.seams = u.seams
	return o
}

// Run does what r asks (ADR 0034), yielding its Progress. With Check it yields whether GitHub
// has a newer stable release (Latest) and takes no other option. Otherwise it walks the running
// binary to the release, from the Upgrader's Source.
func (u *Upgrader) Run(ctx context.Context, r Request) iter.Seq2[Progress, error] {
	return progressOf(func(emit emitter) error { return u.run(ctx, r, emit) })
}

func (u *Upgrader) run(ctx context.Context, r Request, emit emitter) error {
	if r.Check {
		if r.Version != "" || r.Yes {
			return errors.New(Usage)
		}
		latest, err := u.gh.CheckLatest(ctx, buildinfo.Current().Version)
		if err != nil {
			return err
		}
		return emit(latest)
	}
	target, err := os.Executable()
	if err != nil {
		return err
	}
	return walk(ctx, u.bind(Options{
		Target:  target,
		Current: buildinfo.Current().Version,
		Version: r.Version,
		Yes:     r.Yes,
		Service: r.Service,
	}), emit)
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
