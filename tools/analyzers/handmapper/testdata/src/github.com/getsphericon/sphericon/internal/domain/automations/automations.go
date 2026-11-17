// Package automations stands in for a domain module.
package automations

// CreateInput is what the module's Create takes.
type CreateInput struct {
	Name    string
	ActorID int64
}

// Step is part of an input, not an input of its own.
type Step struct{ Kind string }

// ModeInput is named like an input but is no struct.
type ModeInput string

// Default is the module's own literal: a module builds its inputs freely.
var Default = CreateInput{Name: "default"}
