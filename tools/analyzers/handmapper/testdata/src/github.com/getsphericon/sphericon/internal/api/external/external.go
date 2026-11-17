// Package external stands in for the /api handlers.
package external

import "github.com/getsphericon/sphericon/internal/domain/automations"

func create(name string) automations.CreateInput {
	return automations.CreateInput{Name: name} // want `rule 1: automations.CreateInput is built by hand`
}
