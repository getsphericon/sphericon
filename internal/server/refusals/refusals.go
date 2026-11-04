// Package refusals holds the goverter converter of what a refusal names (the Workspaces a User
// solely owns, what uses a row) to the members of the contracts' problem models, which
// refusalProblem in internal/server, the one place a refusal is mapped, renders. The generated
// implementation (ConverterImpl) lives in converter_gen.go.
package refusals

import (
	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/inuse"
)

// Converter maps what a refusal names to the problem models' members. The problem models are
// shared by every contract, so the site's generated types encode them for every surface.
//
// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
type Converter interface {
	// goverter:map PublicID ID
	SoleOwnedWorkspace(source *ent.Workspace) siteapi.SiteSoleOwnedWorkspace
	// goverter:useEmptyOnNil:slice
	SoleOwnedWorkspaces(source []*ent.Workspace) []siteapi.SiteSoleOwnedWorkspace
	// goverter:enum:transform value
	UsedByKind(source inuse.Kind) siteapi.UsedByKind
}
