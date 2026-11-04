// Package staff stands in for ee/staff: the handlers of the Enterprise Staff and service APIs next to
// their domain module.
package staff

import (
	"example/ent"
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

// A handler maps an entity's fields into a resource by hand: a finding.
func (h *ServiceHandlers) lookup(inv *ent.Invitation) any {
	return site.InvitationLookup{Email: inv.Email} // want `rule 1: site.InvitationLookup is built by hand from the fields of ent.Invitation`
}

// The domain module of the package is not a handler.
func (m *Module) lookup(inv *ent.Invitation) any {
	return site.InvitationLookup{Email: inv.Email}
}
