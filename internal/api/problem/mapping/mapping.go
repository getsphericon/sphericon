// Package mapping holds the goverter converter of the problem package (internal/api/problem).
package mapping

import (
	"github.com/getsphericon/sphericon/ent"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
)

// Converter maps what a refusal names to the problem model's members, and the site's problem,
// which Of builds, to the same model of every other surface: each contract emits its own Go types
// of the one ProblemDetails of typespec/common. The generated implementation (ConverterImpl)
// lives in converter_gen.go.
//
// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend externalCode collectCode staffCode staffeeCode serviceCode
type Converter interface {
	// goverter:map PublicID ID
	SoleOwnedWorkspace(source *ent.Workspace) siteapi.SoleOwnedWorkspace
	// goverter:useEmptyOnNil:slice
	SoleOwnedWorkspaces(source []*ent.Workspace) []siteapi.SoleOwnedWorkspace
	// goverter:enum:transform value
	UsedByKind(source inuse.Kind) siteapi.UsedByKind

	External(source *siteapi.ProblemDetails) externalapi.ProblemDetails
	Collect(source *siteapi.ProblemDetails) collectapi.ProblemDetails
	Staff(source *siteapi.ProblemDetails) staffapi.ProblemDetails
	Staffee(source *siteapi.ProblemDetails) staffeeapi.ProblemDetails
	Service(source *siteapi.ProblemDetails) serviceapi.ProblemDetails

	// goverter:enum:transform value
	ExternalCode(source siteapi.ProblemCode) externalapi.ProblemCode
	// goverter:enum:transform value
	CollectCode(source siteapi.ProblemCode) collectapi.ProblemCode
	// goverter:enum:transform value
	StaffCode(source siteapi.ProblemCode) staffapi.ProblemCode
	// goverter:enum:transform value
	StaffeeCode(source siteapi.ProblemCode) staffeeapi.ProblemCode
	// goverter:enum:transform value
	ServiceCode(source siteapi.ProblemCode) serviceapi.ProblemCode

	// goverter:enum:transform value
	ExternalUsedByKind(source siteapi.UsedByKind) externalapi.UsedByKind
	// goverter:enum:transform value
	CollectUsedByKind(source siteapi.UsedByKind) collectapi.UsedByKind
	// goverter:enum:transform value
	StaffUsedByKind(source siteapi.UsedByKind) staffapi.UsedByKind
	// goverter:enum:transform value
	StaffeeUsedByKind(source siteapi.UsedByKind) staffeeapi.UsedByKind
	// goverter:enum:transform value
	ServiceUsedByKind(source siteapi.UsedByKind) serviceapi.UsedByKind
}

// A problem's code is optional: an unset one stays unset, a set one maps member by member.

func externalCode(c Converter, o siteapi.OptProblemCode) externalapi.OptProblemCode {
	return convert.OptVia[externalapi.OptProblemCode](convert.Ptr(o), c.ExternalCode)
}

func collectCode(c Converter, o siteapi.OptProblemCode) collectapi.OptProblemCode {
	return convert.OptVia[collectapi.OptProblemCode](convert.Ptr(o), c.CollectCode)
}

func staffCode(c Converter, o siteapi.OptProblemCode) staffapi.OptProblemCode {
	return convert.OptVia[staffapi.OptProblemCode](convert.Ptr(o), c.StaffCode)
}

func staffeeCode(c Converter, o siteapi.OptProblemCode) staffeeapi.OptProblemCode {
	return convert.OptVia[staffeeapi.OptProblemCode](convert.Ptr(o), c.StaffeeCode)
}

func serviceCode(c Converter, o siteapi.OptProblemCode) serviceapi.OptProblemCode {
	return convert.OptVia[serviceapi.OptProblemCode](convert.Ptr(o), c.ServiceCode)
}
