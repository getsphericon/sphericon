package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/getsphericon/sphericon/internal/kernel/buildinfo"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// Boundaries is what `sphericon migrate --status --json` prints and the walk reads: the
// database's schema state and the binary's table of Majors, oldest first, where the Floor of
// entry N+1 is the last release of Major N (internal/infra/db.Majors).
type Boundaries struct {
	State  MajorFloor   `json:"state"`
	Majors []MajorFloor `json:"majors"`
}

// MajorFloor is one Major and the oldest release that runs on its schema.
type MajorFloor struct {
	Major int    `json:"major"`
	Floor string `json:"floor"`
}

// Walk upgrades to the target (Options.Version, or the newest the Source has), going first
// to the last release of each Major the database has yet to cross, every step a full Step.
// Inside one Major that is a single step.
//
// The table of Majors lives in the release being installed, not in the running binary, so the
// walk stages the target, asks it (`migrate --status --json`, which also reads the database)
// and then installs the Floor of every Major above the database's, oldest first. It needs
// nothing beyond the target's own archive, so it works offline. That same staged binary
// carries the changelog of every release up to itself, so the walk prints the Upgrade notes
// of every release it will install, and waits for --yes, before the first migration or swap;
// the target is then installed from the staged copy rather than fetched again.
//
// With a directory Source every archive of the walk is checked before the first step, and
// a missing one refuses the whole Upgrade. It yields its Progress as it goes.
func Walk(ctx context.Context, o Options) iter.Seq2[Progress, error] {
	return progressOf(func(emit emitter) error { return walk(ctx, o, emit) })
}

func walk(ctx context.Context, o Options, emit emitter) error {
	o, current, err := begin(o)
	if err != nil {
		return err
	}
	target, err := chooseVersion(ctx, o)
	if err != nil {
		return err
	}
	if semver.Compare(buildinfo.Semver(target), current) <= 0 {
		_, err := step(ctx, o, emit) // AlreadyRunning, or the refusal of the downgrade
		return err
	}

	// The staged target: asked for the Majors and the notes, then installed as the last step
	// (the rename takes it away; whatever is left is removed here).
	probe, err := stage(ctx, o, target, o.Target+".plan")
	if err != nil {
		return err
	}
	defer cleanup.Remove(probe)
	b, err := readBoundaries(ctx, o, probe, target)
	if err != nil {
		return err
	}
	plan := planWalk(b, current, target)
	if err := requireArchives(ctx, o, plan); err != nil {
		return err
	}
	if len(plan) > 1 {
		if err := emit(Crossing{Majors: len(plan) - 1, Plan: plan}); err != nil {
			return err
		}
	}
	if err := confirmNotes(ctx, o, probe, current, target, emit); err != nil {
		return err
	}

	for i, version := range plan {
		next := o
		next.Version = version
		next.Current = current
		next.notesConfirmed = true
		if i == len(plan)-1 {
			next.staged = probe
		}
		installed, err := step(ctx, next, emit)
		if err != nil {
			return err
		}
		current = buildinfo.Semver(installed)
	}
	return nil
}

// planWalk is the releases to install, oldest first, ending with the target: the Floor of
// every Major above the database's that lies between the running release and the target.
func planWalk(b Boundaries, current, target string) []string {
	var plan []string
	for _, m := range b.Majors {
		if m.Major <= b.State.Major || !semver.IsValid(m.Floor) {
			continue
		}
		if semver.Compare(m.Floor, current) > 0 && semver.Compare(m.Floor, buildinfo.Semver(target)) < 0 {
			plan = append(plan, buildinfo.Bare(m.Floor))
		}
	}
	return append(plan, target)
}

// requireArchives refuses, naming every release, when the Source lacks one the walk needs.
func requireArchives(ctx context.Context, o Options, plan []string) error {
	have, err := o.Source.Versions(ctx, o.Platform)
	if err != nil {
		return err
	}
	var missing []string
	for _, v := range plan {
		if !slices.Contains(have, v) {
			missing = append(missing, ArchiveName(v, o.Platform))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("this Upgrade needs %d archive(s) the source does not have (nothing was changed): %s",
			len(missing), strings.Join(missing, ", "))
	}
	return nil
}

// readBoundaries asks the staged target which Majors lie between.
func readBoundaries(ctx context.Context, o Options, staged, target string) (Boundaries, error) {
	out, err := o.Run(ctx, staged, []string{"migrate", "--status", "--json"})
	if err != nil {
		return Boundaries{}, fmt.Errorf("ask %s which Majors lie between (nothing was changed): %w", target, err)
	}
	var b Boundaries
	if err := json.Unmarshal(out, &b); err != nil {
		return Boundaries{}, errors.Join(errors.New("read the Majors from the new binary"), err)
	}
	return b, nil
}
