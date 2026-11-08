package upgrade

import (
	"errors"
	"iter"
)

// Progress is one thing an Upgrade found or did, yielded as it happens so the command line can
// print it before the next, longer, part runs (ADR 0111: the domain tier writes to no io.Writer).
// It is a Latest, an AlreadyRunning, a Crossing, a Notes, a Migrated or an Installed.
type Progress interface{ progress() }

// Latest is what `upgrade --check` found: the newest stable release, the running version and
// whether that release is newer.
type Latest struct {
	Release string
	Running string
	Newer   bool
}

// AlreadyRunning: the Instance runs the release already, and nothing was changed.
type AlreadyRunning struct{ Version string }

// Crossing: the walk installs the release Plan in turn, the last release of each of Majors
// Majors and then the target.
type Crossing struct {
	Majors int
	Plan   []string
}

// Notes are the Upgrade notes of every release after From up to To; without --yes the Upgrade
// stops after them with ErrNeedsConfirmation.
type Notes struct {
	From, To string
	Text     string
}

// Migrated: the release Version migrated the database; Output is what its `migrate` printed.
type Migrated struct {
	Version string
	Output  string
}

// Installed: a release replaced the binary. Restarted says whether the systemd unit Service was
// restarted into it (`--restart`); otherwise restarting it is left to the Self-hoster.
type Installed struct {
	Result
	Service   string
	Restarted bool
}

func (Latest) progress()         {}
func (AlreadyRunning) progress() {}
func (Crossing) progress()       {}
func (Notes) progress()          {}
func (Migrated) progress()       {}
func (Installed) progress()      {}

// emitter hands one Progress to the caller; it fails with errStopped once the caller stops
// reading.
type emitter func(Progress) error

// errStopped: the caller stopped reading the Progress, so the Upgrade stops where it is.
var errStopped = errors.New("upgrade: the caller stopped reading")

// progressOf runs an Upgrade, yielding each Progress it emits and then the error it ended with.
func progressOf(run func(emit emitter) error) iter.Seq2[Progress, error] {
	return func(yield func(Progress, error) bool) {
		err := run(func(p Progress) error {
			if !yield(p, nil) {
				return errStopped
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStopped) {
			yield(nil, err)
		}
	}
}
