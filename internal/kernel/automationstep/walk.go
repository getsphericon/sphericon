package automationstep

import "iter"

// The walker is the one way a reader visits a version's stored Steps: the executor, the version
// view and its write-time merge, the send Steps, secret rotation and Definition use go through it
// and never index into the decoded Steps. A Step's position is where it stands in the version, 0
// for the first; the Step tree changes this walker, not its readers.

// All yields each Step of the version with its position, in the order an Enrollment reaches them.
// The Step is the version's own: a reader does not change it.
func All(steps []Step) iter.Seq2[int, *Step] {
	return func(yield func(int, *Step) bool) {
		for i := range steps {
			if !yield(i, &steps[i]) {
				return
			}
		}
	}
}

// At is the Step at the position, false when the version has none there: before its first Step or
// past its last, where an Enrollment has finished.
func At(steps []Step, pos int) (*Step, bool) {
	for i, st := range All(steps) {
		if i == pos {
			return st, true
		}
	}
	return nil, false
}

// Map returns the version's Steps with each one, in order, replaced by what rewrite makes of it,
// each in the place of the Step it was made from. The Steps given are left as they are, and no
// Steps map to an empty list, never nil, since a version stores `[]`. The first error stops the
// walk and is returned.
func Map(steps []Step, rewrite func(pos int, st *Step) (Step, error)) ([]Step, error) {
	out := make([]Step, 0, len(steps))
	for pos, st := range All(steps) {
		made, err := rewrite(pos, st)
		if err != nil {
			return nil, err
		}
		out = append(out, made)
	}
	return out, nil
}

// Rewrite is Map for a rewrite that cannot fail.
func Rewrite(steps []Step, rewrite func(pos int, st *Step) Step) []Step {
	out := make([]Step, 0, len(steps))
	for pos, st := range All(steps) {
		out = append(out, rewrite(pos, st))
	}
	return out
}
