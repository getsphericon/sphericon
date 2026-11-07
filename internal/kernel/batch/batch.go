// Package batch is the one partial-success shape of a use case's batch (ADR 0104): each item
// succeeds or fails on its own, and the caller reads what became of each item in the order it gave.
package batch

// Item is what became of one item of a batch: its Value, or the Err that refused or failed it.
type Item[T any] struct {
	Value T
	Err   error
}

// Result is a batch's items, one per item given, in the order they were given.
type Result[T any] struct {
	Items []Item[T]
}

// Each runs do for every item in order and keeps what became of each; a failing item stops
// nothing.
func Each[I, T any](items []I, do func(I) (T, error)) Result[T] {
	out := make([]Item[T], len(items))
	for i, item := range items {
		out[i].Value, out[i].Err = do(item)
	}
	return Result[T]{Items: out}
}
