// Package staff stands in for ee/staff: the handlers of the Enterprise Staff and service APIs next to
// their domain module.
package staff

import (
	"example/gen/site"
	"example/internal/automations"
)

type ServiceHandlers struct{ module *Module }

type Module struct{}

// A handler builds the module's input from the request: a finding.
func (h *ServiceHandlers) create(req *site.CreateAutomationInput) automations.CreateInput {
	return automations.CreateInput{Name: req.Name} // want `rule 1: automations.CreateInput is built by hand from the request`
}

// The domain module of the package builds another module's input freely.
func (m *Module) create(name string) automations.CreateInput {
	return automations.CreateInput{Name: name}
}

// A function outside the handler types belongs to the module.
func defaults() automations.CreateInput {
	return automations.CreateInput{Name: "default"}
}
