package automationstep

import "iter"

// The walker is the one way a reader visits a version's stored Steps: no reader indexes into the
// decoded Steps, so a change of their shape changes this walker, not its readers. A Step's
// position is where it stands in the version, 0 for the first.

// All yields each Step of the path with its position, in the order an Enrollment reaches them.
// The Step is the version's own: a reader does not change it.
func All(p Path) iter.Seq2[int, *Step] {
	return func(yield func(int, *Step) bool) {
		for i := range p.Steps {
			if !yield(i, &p.Steps[i]) {
				return
			}
		}
	}
}

// First is the first Step of the path, where an Enrollment begins; false for a path of no Steps.
func First(p Path) (*Step, bool) {
	for _, st := range All(p) {
		return st, true
	}
	return nil, false
}

// Find is the Step of the id, false when the path holds none: an Enrollment names the Step it
// stands on by its id, which every version keeps (ADR 0122).
func Find(p Path, id string) (*Step, bool) {
	for _, st := range All(p) {
		if st.ID == id {
			return st, true
		}
	}
	return nil, false
}

// Next is the Step an Enrollment reaches after the one of the id: the following Step of the path.
// False after the last Step, where the Enrollment completes, and for an id the path does not hold.
func Next(p Path, id string) (*Step, bool) {
	found := false
	for _, st := range All(p) {
		if found {
			return st, true
		}
		found = st.ID == id
	}
	return nil, false
}

// AfterEnd is the first Step that follows an End in its path, false when every End is the last
// Step of its path (ADR 0124): an End finishes its path, so a Step after it would never run.
func AfterEnd(p Path) (*Step, bool) {
	ended := false
	for _, st := range All(p) {
		if ended {
			return st, true
		}
		ended = st.Kind() == KindEnd
	}
	return nil, false
}

// Map returns the path with each Step, in order, replaced by what rewrite makes of it, each in the
// place of the Step it was made from; the path keeps its id. Map writes nothing into the path
// given, and rewrite is handed the version's own Step, which it does not change either. No Steps
// map to an empty list, never nil, since a path stores `[]`. The first error stops the walk and is
// returned, with the zero Path.
func Map(p Path, rewrite func(pos int, st *Step) (Step, error)) (Path, error) {
	out := make([]Step, 0, len(p.Steps))
	for pos, st := range All(p) {
		made, err := rewrite(pos, st)
		if err != nil {
			return Path{}, err
		}
		out = append(out, made)
	}
	return Path{ID: p.ID, Steps: out}, nil
}

// Rewrite is Map for a rewrite that cannot fail.
func Rewrite(p Path, rewrite func(pos int, st *Step) Step) Path {
	out := make([]Step, 0, len(p.Steps))
	for pos, st := range All(p) {
		out = append(out, rewrite(pos, st))
	}
	return Path{ID: p.ID, Steps: out}
}
