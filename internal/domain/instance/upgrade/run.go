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

// Run does what r asks (ADR 0034), yielding its Progress. With Check it yields whether GitHub
// has a newer stable release (Latest) and takes no other option. Otherwise it walks the running
// binary to the release, from the directory From or from GitHub.
func Run(ctx context.Context, r Request) iter.Seq2[Progress, error] {
	return progressOf(func(emit emitter) error { return r.run(ctx, emit) })
}

func (r Request) run(ctx context.Context, emit emitter) error {
	if r.Check {
		if r.From != "" || r.Version != "" || r.Yes || r.Restart {
			return errors.New(Usage)
		}
		gh, err := NewGitHub()
		if err != nil {
			return err
		}
		latest, err := CheckLatest(ctx, gh, buildinfo.Current().Version)
		if err != nil {
			return err
		}
		return emit(latest)
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
		Service: r.Service,
	}
	if r.Restart {
		sd := Systemd{Service: r.Service}
		opts.Run, opts.PostSwap = sd.Run, sd.PostSwap
	}
	return walk(ctx, opts, emit)
}

func (r Request) source() (Source, error) {
	if r.From != "" {
		return DirSource(r.From), nil
	}
	return NewGitHub()
}

// CheckLatest says whether gh has a stable release newer than current, changing nothing.
func CheckLatest(ctx context.Context, gh *GitHub, current string) (Latest, error) {
	latest, err := gh.LatestRelease(ctx)
	if err != nil {
		return Latest{}, err
	}
	return Latest{
		Release: latest,
		Running: current,
		Newer:   semver.Compare(buildinfo.Semver(latest), buildinfo.Semver(current)) > 0,
	}, nil
}
