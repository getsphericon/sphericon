// Package batch is the one partial-success shape of a use case's batch (ADR 0104): each item
// succeeds or fails on its own, and the caller reads one outcome per item in the order it gave.
package batch

// Outcome is what became of one item: its Value, or the Err that refused or failed it.
type Outcome[T any] struct {
	Value T
	Err   error
}

// Result is a batch's outcomes, one per item, in the order the items were given.
type Result[T any] struct {
	Outcomes []Outcome[T]
}

// Each runs do for every item in order and keeps each outcome; a failing item stops nothing.
func Each[I, T any](items []I, do func(I) (T, error)) Result[T] {
	out := make([]Outcome[T], len(items))
	for i, item := range items {
		out[i].Value, out[i].Err = do(item)
	}
	return Result[T]{Outcomes: out}
}
