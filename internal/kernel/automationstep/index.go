package automationstep

// Index is where each Step and path of a version's tree stands (ADR 0122, ADR 0124): a Step's
// path and its place there, a path's fork. The executor builds it once from the version it decoded
// and asks it where an Enrollment goes next, so a reader never walks the tree for a move.
type Index struct {
	steps map[ID]stepPlace
	paths map[ID]pathPlace
}

// stepPlace is a Step, the path that holds it and its place among that path's Steps.
type stepPlace struct {
	step *Step
	path ID
	pos  int
}

// pathPlace is a path's Steps and the fork that holds it, "" for the root.
type pathPlace struct {
	steps []Step
	fork  ID
}

// NewIndex indexes the tree of the root path p. The Steps it hands out are the tree's own.
func NewIndex(p Path) *Index {
	x := &Index{steps: map[ID]stepPlace{}, paths: map[ID]pathPlace{}}
	x.add(p, "")
	return x
}

func (x *Index) add(p Path, fork ID) {
	x.paths[p.ID] = pathPlace{steps: p.Steps, fork: fork}
	for i := range p.Steps {
		st := &p.Steps[i]
		x.steps[st.ID] = stepPlace{step: st, path: p.ID, pos: i}
		for _, sub := range Forks(st) {
			x.add(sub, st.ID)
		}
	}
}

// Step is the Step of the id, false when the tree holds none: an Enrollment names the Step it
// stands on by its id, which every version keeps (ADR 0122).
func (x *Index) Step(id ID) (*Step, bool) {
	at, ok := x.steps[id]
	return at.step, ok
}

// Next is the Step an Enrollment reaches after the one of the id: the following Step of its path
// or, after the last Step of a path, the Step after the fork that holds the path, recursively.
// False after the last Step of the root, where the Enrollment completes, and for an id the tree
// does not hold.
func (x *Index) Next(id ID) (*Step, bool) {
	at, ok := x.steps[id]
	if !ok {
		return nil, false
	}
	path := x.paths[at.path]
	if at.pos+1 < len(path.steps) {
		return &path.steps[at.pos+1], true
	}
	if path.fork == "" {
		return nil, false
	}
	return x.Next(path.fork)
}

// Enter is the Step an Enrollment reaches when it takes the path of the id: its first Step or, for
// an empty path, the Step after its fork (Next of the fork). False for an id no path of the tree
// holds, and when nothing follows the empty path's fork.
func (x *Index) Enter(path ID) (*Step, bool) {
	p, ok := x.paths[path]
	if !ok {
		return nil, false
	}
	if len(p.steps) > 0 {
		return &p.steps[0], true
	}
	if p.fork == "" {
		return nil, false
	}
	return x.Next(p.fork)
}
