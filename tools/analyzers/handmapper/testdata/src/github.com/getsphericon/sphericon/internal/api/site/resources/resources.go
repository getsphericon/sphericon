// Package resources stands in for a surface's goverter home: its helpers may build an input.
package resources

import "github.com/getsphericon/sphericon/internal/domain/automations"

func createIn(name string) automations.CreateInput { return automations.CreateInput{Name: name} }

var _ = createIn
