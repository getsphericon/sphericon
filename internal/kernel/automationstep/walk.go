package automationstep

import (
	"iter"
	"slices"
	"strconv"
)

// The walker is the one way a reader visits a version's stored Steps: no reader indexes into the
// decoded Steps, so a change of their shape changes this walker, not its readers. A version's Steps
// are a tree (ADR 0122, ADR 0124): the root path, whose forks hold paths of their own that rejoin
// after the fork. The walker visits it in pre-order: each Step, then the Steps of its paths in the
// order the fork gives them (Fork.Forks), then the Step after it. A Step's position is its place
// in that order, 0 for the first.

// All yields each Step of the tree with its position, in pre-order: a fork, then the Steps of its
// paths, then the Step after the fork. The Step is the version's own: a reader does not change it.
func All(p Path) iter.Seq2[int, *Step] {
	return func(yield func(int, *Step) bool) {
		pos := 0
		walk(p, &pos, yield)
	}
}

// walk yields the Steps of the path p and of the paths of its forks, counting their positions on
// from pos; false when yield stopped the walk.
func walk(p Path, pos *int, yield func(int, *Step) bool) bool {
	for i := range p.Steps {
		st := &p.Steps[i]
		if !yield(*pos, st) {
			return false
		}
		*pos++
		for _, sub := range Forks(st) {
			if !walk(sub, pos, yield) {
				return false
			}
		}
	}
	return true
}

// Own yields the Steps of the path p itself, in order, without the Steps of its forks' paths: what
// a reader that keeps the tree's shape (a view of it) walks, reaching the paths of a fork through
// Forks.
func Own(p Path) iter.Seq[*Step] {
	return func(yield func(*Step) bool) {
		for i := range p.Steps {
			if !yield(&p.Steps[i]) {
				return
			}
		}
	}
}

// Forks are the paths of the Step when it is a fork, in the order the walker visits them; none for
// any other Step.
func Forks(st *Step) []Path {
	if f, ok := st.Payload.(Fork); ok {
		return forkPaths(f)
	}
	return nil
}

// Locations are where each Step of the tree lies in the path's document, as a request writes it,
// by its id: `steps.1` for the second Step of the path, `steps.1.paths.0.steps.0` for the first of
// the first listed path of that fork, `steps.1.otherwise.steps.0` in a Branch's otherwise. A
// refusal of a Step is placed there.
func Locations(p Path) map[ID]string {
	out := map[ID]string{}
	locate(p, "", out)
	return out
}

// locate adds the location of each Step of the path p and of its forks' paths to out, each under
// the prefix of p's own location ("" for the root).
func locate(p Path, prefix string, out map[ID]string) {
	for i := range p.Steps {
		st := &p.Steps[i]
		at := prefix + "steps." + strconv.Itoa(i)
		out[st.ID] = at
		f, ok := st.Payload.(Fork)
		if !ok {
			continue
		}
		for _, sub := range f.Forks() {
			locate(sub.Path, at+"."+sub.Place+".", out)
		}
	}
}

// Paths yields every path of the tree: the root, then the paths of each fork in the order the
// walker reaches them.
func Paths(p Path) iter.Seq[Path] {
	return func(yield func(Path) bool) {
		walkPaths(p, yield)
	}
}

func walkPaths(p Path, yield func(Path) bool) bool {
	if !yield(p) {
		return false
	}
	for i := range p.Steps {
		for _, sub := range Forks(&p.Steps[i]) {
			if !walkPaths(sub, yield) {
				return false
			}
		}
	}
	return true
}

// First is the first Step of the root path, where an Enrollment begins; false for a path of no
// Steps.
func First(p Path) (*Step, bool) {
	for st := range Own(p) {
		return st, true
	}
	return nil, false
}

// AfterEnd is the first Step that follows an End in its path, false when every End is the last
// Step of its path (ADR 0124): an End finishes its path, so a Step after it would never run. Each
// path is checked on its own: an End finishes the path that holds it, not the path around its fork.
func AfterEnd(p Path) (*Step, bool) {
	for path := range Paths(p) {
		if st, ok := afterEndIn(path); ok {
			return st, true
		}
	}
	return nil, false
}

// afterEndIn is the first Step that follows an End among the Steps of the path p itself.
func afterEndIn(p Path) (*Step, bool) {
	ended := false
	for st := range Own(p) {
		if ended {
			return st, true
		}
		ended = st.Kind() == KindEnd
	}
	return nil, false
}

// Map returns the tree with each Step, in the walker's order, replaced by what rewrite makes of it,
// each in the place of the Step it was made from; every path keeps its id. The paths of a fork
// rewrite made are rewritten in turn, so rewrite reaches every Step of the tree, and a fork keeps
// what it holds besides its paths (Fork.WithForks). Map writes nothing into the tree given, and
// rewrite is handed the version's own Step, which it does not change either. No Steps map to an
// empty list, never nil, since a path stores `[]`. The first error stops the walk and is returned,
// with the zero Path.
func Map(p Path, rewrite func(pos int, st *Step) (Step, error)) (Path, error) {
	pos := 0
	return mapPath(p, &pos, rewrite)
}

func mapPath(p Path, pos *int, rewrite func(pos int, st *Step) (Step, error)) (Path, error) {
	out := make([]Step, 0, len(p.Steps))
	for i := range p.Steps {
		made, err := rewrite(*pos, &p.Steps[i])
		if err != nil {
			return Path{}, err
		}
		*pos++
		if f, ok := made.Payload.(Fork); ok {
			subs := forkPaths(f)
			mapped := make([]Path, len(subs))
			for j, sub := range subs {
				if mapped[j], err = mapPath(sub, pos, rewrite); err != nil {
					return Path{}, err
				}
			}
			made.Payload = f.WithForks(mapped)
		}
		out = append(out, made)
	}
	return Path{ID: p.ID, Steps: out}, nil
}

// Rewrite is Map for a rewrite that cannot fail.
func Rewrite(p Path, rewrite func(pos int, st *Step) Step) Path {
	out, err := Map(p, func(pos int, st *Step) (Step, error) { return rewrite(pos, st), nil })
	if err != nil {
		return Path{} // rewrite returns no error
	}
	return out
}

// forkPaths are the paths of the fork, in the order Forks gives them, without their places.
func forkPaths(f Fork) []Path {
	placed := f.Forks()
	out := make([]Path, len(placed))
	for i, p := range placed {
		out[i] = p.Path
	}
	return out
}

// Before are the Steps an Enrollment may have passed when it reaches the Step at (ADR 0122), in the
// walker's order: those before it in its own path and in every path around it, each with every Step
// of its forks' paths, whichever of them the Enrollment took, and the forks it stands in. It never
// holds the Step at itself, a Step after it, one of its own paths, or one of another path of a fork
// it stands in, which an Enrollment on its path never passed. None when the tree has no Step at.
func Before(p Path, at ID) []*Step {
	steps, _ := before(p, at, nil)
	return steps
}

// before is Before within the path p, passed is what the Enrollment may have passed when it entered
// p; false when p holds no Step at, at any depth.
func before(p Path, at ID, passed []*Step) ([]*Step, bool) {
	for i := range p.Steps {
		st := &p.Steps[i]
		if st.ID == at {
			return passed, true
		}
		into := append(slices.Clip(passed), st)
		for _, sub := range Forks(st) {
			if found, ok := before(sub, at, into); ok {
				return found, true
			}
		}
		passed = into
		for _, sub := range Forks(st) {
			for _, inner := range All(sub) {
				passed = append(passed, inner)
			}
		}
	}
	return nil, false
}
