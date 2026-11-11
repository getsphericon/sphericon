package schema

import "entgo.io/ent/schema"

// Form is the form a state field takes (ADR 0109, review rule 7).
type Form string

const (
	// FormMachine is a state that moves back and forth: the ADR 0046 lifecycle table with its
	// compare-and-swap write, in the module that owns the entity. Its column is written only by the
	// Write of a `lifecycle.Storage`.
	FormMachine Form = "machine"
	// FormLatch is a fact recorded once and never cleared. Its column is written only inside
	// `lifecycle.Latch`, conditioned on the moment still being null.
	FormLatch Form = "latch"
	// FormAttribute is not a state, or a state no move depends on; its State says why (Reason).
	FormAttribute Form = "attribute"
)

// State declares the form of a state field (ADR 0109). Every field that ends in `_at`, every
// `status` or `state` enum and every bool carries one (TestEveryStateFieldDeclaresItsForm), and so
// may any other field that is a state (a Tracking domain's `role`). ent/entc.go generates the
// machine and latch fields into package ent as StateForm<Entity><Field> constants, and the
// `statemoment` analyzer refuses an update of one outside the lifecycle primitives.
type State struct {
	Form Form
	// Reason says why an attribute is one; a machine or a latch needs none.
	Reason string
}

func (State) Name() string { return "State" }

var _ schema.Annotation = State{}

// Machine declares a field written only by a `lifecycle` machine's compare-and-swap.
func Machine() State { return State{Form: FormMachine} }

// Latch declares a fact recorded once through `lifecycle.Latch` and never cleared.
func Latch() State { return State{Form: FormLatch} }

// Attribute declares a field that no move is conditioned on, and why: a moment that only records
// history, a setting its owner edits, the latest observation.
func Attribute(reason string) State { return State{Form: FormAttribute, Reason: reason} }

// The reasons an attribute gives most often.
const (
	reasonHistory    = "records when something happened; no move is conditioned on it"
	reasonWithStatus = "records a move of the row's status, written in the same compare-and-swap by its machine"
	reasonExpiry     = "an expiry fixed when the row is written; a read compares it with the clock"
	reasonLatest     = "the latest observation, overwritten by the next one"
	reasonEarliest   = "the earliest observation: a report only ever moves it earlier"
	reasonSetting    = "a setting its owner edits"
	reasonFact       = "a fact of the row, fixed when it is written"
	reasonBucket     = "the token bucket's refill clock, moved by every take"
	reasonNotice     = "when the last notice went out, so the next one is not sent twice"
	reasonFoundRun   = "an observation: the first check of a run that found it, cleared by the check that finds otherwise; it gates no move"
)
