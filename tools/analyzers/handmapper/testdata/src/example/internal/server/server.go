// Package server stands in for internal/server, which renders the refusals with the API's types.
package server

import (
	"example/ent"
	"example/gen/site"
)

// refused stands in for a module's error that carries entities.
type refused struct{ Workspaces []*ent.Workspace }

func owned(e *refused) []site.SoleOwnedWorkspace {
	out := []site.SoleOwnedWorkspace{}
	for _, w := range e.Workspaces {
		out = append(out, site.SoleOwnedWorkspace{ID: site.WorkspaceID(w.PublicID), Name: w.Name}) // want `rule 1: site.SoleOwnedWorkspace is built by hand from the fields of ent.Workspace`
	}
	return out
}

// A problem built from plain values reads no entity.
func problem(detail string) site.DeleteProblem { return site.DeleteProblem{Detail: detail} }
