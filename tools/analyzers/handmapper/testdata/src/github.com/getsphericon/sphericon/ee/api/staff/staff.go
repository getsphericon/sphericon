// Package staff stands in for ee/api/staff: the handlers of the Enterprise Staff and service APIs,
// checked like the handlers of every other surface.
package staff

import (
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/automations"
)

type ServiceHandlers struct{}

// A handler builds the module's input from the request: a finding.
func (h *ServiceHandlers) create(req *site.CreateAutomationInput) automations.CreateInput {
	return automations.CreateInput{Name: req.Name} // want `rule 1: automations.CreateInput is built by hand from the request`
}

// A handler maps an entity's fields into a resource by hand: a finding.
func (h *ServiceHandlers) lookup(inv *ent.Invitation) any {
	return site.InvitationLookup{Email: inv.Email} // want `rule 1: site.InvitationLookup is built by hand from the fields of ent.Invitation`
}
