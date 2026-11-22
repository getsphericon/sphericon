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

// At is the Step at the position, false when the version has none there: before its first Step or
// past its last, where an Enrollment has finished.
func At(p Path, pos int) (*Step, bool) {
	for i, st := range All(p) {
		if i == pos {
			return st, true
		}
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
