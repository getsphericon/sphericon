// Package site stands in for the /site handlers.
package site

import (
	"example/gen/site"
	"example/internal/automations"
)

func create(req *site.CreateAutomationInput) automations.CreateInput {
	return automations.CreateInput{Name: req.Name} // want `rule 1: automations.CreateInput is built by hand from the request; fix: .*goverter Inputs`
}

func createPtr(req *site.CreateAutomationInput) *automations.CreateInput {
	return &automations.CreateInput{Name: req.Name} // want `rule 1: automations.CreateInput is built by hand`
}

func batch(names []string) []automations.CreateInput {
	return []automations.CreateInput{{Name: names[0]}} // want `rule 1: automations.CreateInput is built by hand`
}

// Inputs stands in for the generated Inputs.
type Inputs interface {
	AutomationCreate(req *site.CreateAutomationInput) automations.CreateInput
}

var inputs Inputs

// generated maps through Inputs and assigns the field it ignores: no literal.
func generated(req *site.CreateAutomationInput, actor int64) automations.CreateInput {
	in := inputs.AutomationCreate(req)
	in.ActorID = actor
	return in
}

// A part of an input, a non-struct ...Input and the request itself are no module inputs.
var (
	step = automations.Step{Kind: "email"}
	mode = automations.ModeInput("x")
	body = site.CreateAutomationInput{Name: "x"}
)

// formInput is the handler's own type.
type formInput struct{ Email string }

var form = formInput{Email: "a@b"}
