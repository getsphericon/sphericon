package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// Boundaries is what `sphericon migrate --status --json` prints and the walk reads: the
// database's schema state and the binary's table of Majors, oldest first, where the Floor of
// entry N+1 is the last release of Major N (internal/db.Majors).
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
// nothing beyond the target's own archive, so it works offline.
//
// With a directory Source every archive of the walk is checked before the first step, and
// a missing one refuses the whole Upgrade.
func Walk(ctx context.Context, o Options) ([]Result, error) {
	if o.Platform == (Platform{}) {
		o.Platform = Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}
	}
	if o.Container == nil {
		o.Container = DetectContainer
	}
	if o.Run == nil {
		o.Run = execRun
	}
	if kind := o.Container(); kind != "" {
		return nil, containerRefusal(kind)
	}
	current := "v" + strings.TrimPrefix(o.Current, "v")
	if !semver.IsValid(current) {
		return nil, fmt.Errorf("this build is %q, not a release: install a release binary first", o.Current)
	}
	target, err := chooseVersion(ctx, o)
	if err != nil {
		return nil, err
	}
	if semver.Compare("v"+target, current) <= 0 {
		res, err := Step(ctx, o) // prints "Already running" or refuses the downgrade
		return []Result{res}, err
	}

	b, err := readBoundaries(ctx, o, target)
	if err != nil {
		return nil, err
	}
	plan := planWalk(b, current, target)
	if err := requireArchives(ctx, o, plan); err != nil {
		return nil, err
	}
	if len(plan) > 1 {
		if _, err := fmt.Fprintf(o.Out, "Crossing %d Majors: installing %s in turn.\n", len(plan)-1, strings.Join(plan, ", ")); err != nil {
			return nil, err
		}
	}

	var results []Result
	for _, version := range plan {
		step := o
		step.Version = version
		step.Current = current
		res, err := Step(ctx, step)
		if err != nil {
			return results, err
		}
		results = append(results, res)
		current = "v" + res.Version
	}
	return results, nil
}

// planWalk is the releases to install, oldest first, ending with the target: the Floor of
// every Major above the database's that lies between the running release and the target.
func planWalk(b Boundaries, current, target string) []string {
	var plan []string
	for _, m := range b.Majors {
		if m.Major <= b.State.Major || !semver.IsValid(m.Floor) {
			continue
		}
		if semver.Compare(m.Floor, current) > 0 && semver.Compare(m.Floor, "v"+target) < 0 {
			plan = append(plan, strings.TrimPrefix(m.Floor, "v"))
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

// readBoundaries stages the target next to the installed binary just long enough to ask it.
func readBoundaries(ctx context.Context, o Options, target string) (Boundaries, error) {
	rel, err := o.Source.Fetch(ctx, target, o.Platform)
	if err != nil {
		return Boundaries{}, err
	}
	if rel.Cleanup != nil {
		defer rel.Cleanup()
	}
	if err := verify(rel); err != nil {
		return Boundaries{}, err
	}
	probe := o.Target + ".plan"
	defer func() { _ = os.Remove(probe) }()
	if err := unpack(rel.ArchivePath, probe); err != nil {
		return Boundaries{}, err
	}
	var out bytes.Buffer
	if err := o.Run(ctx, probe, []string{"migrate", "--status", "--json"}, &out); err != nil {
		return Boundaries{}, fmt.Errorf("ask %s which Majors lie between (nothing was changed): %w", target, err)
	}
	var b Boundaries
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		return Boundaries{}, errors.Join(errors.New("read the Majors from the new binary"), err)
	}
	return b, nil
}
