// Package lifecycle is the one way an entity's states are declared (ADR 0046): a thin
// constructor over github.com/qmuntal/stateless with external storage. The transition table
// (Configure(...).Permit(...)) stays in the module that owns the entity; this package gives it
// the storage seam and the single error.
//
// A move fails with ErrInvalidTransition in two cases, and callers handle one: the trigger is
// not permitted from the state the machine read, or the storage write reports a compare-and-swap
// miss (the row changed after it was read). A worker treats the error as a stale event and
// finishes the job without a retry; an API handler answers 409. It is never swallowed.
//
// Exit actions carry no side effects. stateless runs them before the write and does not roll
// them back, so an exit action that sends mail or enqueues a job would do so for a move that
// the write then refuses. Put effects in the same transaction as the write, after the move.
package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/qmuntal/stateless"
)

// ErrInvalidTransition is returned by a fire call that no legal move satisfies: an unpermitted
// trigger, or a write that lost the compare-and-swap.
var ErrInvalidTransition = errors.New("invalid transition")

// Storage is where an entity's state lives. Write is the compare-and-swap: a conditional
// update that stores to only while the row is still in from (the state Read returned), and
// reports whether it did. A miss is (false, nil); a database fault is the error.
type Storage[S comparable] struct {
	Read  func(ctx context.Context) (S, error)
	Write func(ctx context.Context, from, to S) (swapped bool, err error)
}

// New returns a state machine over the storage, with no states configured. Build one per
// entity use: it remembers the state it read to make the write conditional on it.
func New[S comparable](storage Storage[S]) *stateless.StateMachine {
	var read S
	accessor := func(ctx context.Context) (stateless.State, error) {
		state, err := storage.Read(ctx)
		read = state
		return state, err
	}
	mutator := func(ctx context.Context, to stateless.State) error {
		target, ok := to.(S)
		if !ok {
			return fmt.Errorf("lifecycle: state %v is not of the storage's type", to)
		}
		swapped, err := storage.Write(ctx, read, target)
		if err != nil {
			return err
		}
		if !swapped {
			return fmt.Errorf("%w: %v is no longer the state", ErrInvalidTransition, read)
		}
		read = target
		return nil
	}
	m := stateless.NewStateMachineWithExternalStorage(accessor, mutator, stateless.FiringImmediate)
	m.OnUnhandledTrigger(func(_ context.Context, state stateless.State, trigger stateless.Trigger, _ []string) error {
		return fmt.Errorf("%w: %v is not permitted from %v", ErrInvalidTransition, trigger, state)
	})
	return m
}
