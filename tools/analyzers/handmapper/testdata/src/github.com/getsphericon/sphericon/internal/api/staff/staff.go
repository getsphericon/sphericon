// Package staff stands in for the core /staff handlers.
package staff

import (
	"github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/automations"
)

func create(req *site.CreateAutomationInput) automations.CreateInput {
	return automations.CreateInput{Name: req.Name} // want `rule 1: automations.CreateInput is built by hand from the request`
}
