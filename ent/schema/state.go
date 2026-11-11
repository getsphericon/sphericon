package schema

import "entgo.io/ent/schema"

// Form is the form a state field takes (ADR 0109).
type Form string

const (
	// FormMachine is a state that moves back and forth: the ADR 0046 lifecycle table with its
	// compare-and-swap write, in the module that owns the entity.
	FormMachine Form = "machine"
	// FormLatch is a moment recorded once and never cleared: one conditional write through
	// lifecycle.Latch, which matches only while the moment is null.
	FormLatch Form = "latch"
	// FormAttribute is a field of state's shape that is neither: State.Reason says why.
	FormAttribute Form = "attribute"
)

// State declares the form of a state field (ADR 0109): a moment (`<x>_at`), a `status` or `state`
// enum, or a bool. An attribute names its Reason.
type State struct {
	Form   Form
	Reason string
}

func (State) Name() string { return "State" }

var _ schema.Annotation = State{}
