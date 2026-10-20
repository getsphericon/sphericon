package emaildoc

// Sustained is the memory guard's run-of-polls counter, exposed to the black-box tests.
type Sustained = sustained

// NewSustained counts `need` polls in a row.
func NewSustained(need int) *Sustained { return &sustained{need: need} }

// Observe records one poll.
func (s *sustained) Observe(over bool) bool { return s.observe(over) }

// Prepare runs the checks a document passes before a runtime is needed.
func Prepare(r *Renderer, doc []byte) error { return r.prepare(doc) }
