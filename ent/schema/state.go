package schema

import "entgo.io/ent/schema"

// Form is the form a state field takes (ADR 0109).
type Form string

// FormMachine is a state that moves back and forth: the ADR 0046 lifecycle table with its
// compare-and-swap write, in the module that owns the entity.
const FormMachine Form = "machine"

// State declares the form of a state field (ADR 0109): a moment (`<x>_at`), a `status` or `state`
// enum, or a bool.
type State struct {
	Form Form
}

func (State) Name() string { return "State" }

var _ schema.Annotation = State{}
