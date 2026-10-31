// Package external stands in for the /api handlers.
package external

import "example/internal/automations"

func create(name string) automations.CreateInput {
	return automations.CreateInput{Name: name} // want `rule 1: automations.CreateInput is built by hand`
}
