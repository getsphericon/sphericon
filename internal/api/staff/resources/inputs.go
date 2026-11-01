package resources

import (
	"github.com/getsphericon/sphericon/ent/signupitem"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/signupitems"
)

// Inputs maps request bodies onto the modules' inputs: the input direction of the contract, so a
// handler only hands the module what the request says. The extends below bind the generated option
// types of this surface to the shared conversions of internal/convert.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name InputsImpl
// goverter:output:file ./inputs_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend boolIn
// goverter:extend boolPtrIn
// goverter:extend titlePtrIn
type Inputs interface {
	SignupItemCreate(source *staffapi.StaffSignupItemInput) signupitems.Input
	SignupItemUpdate(source *staffapi.StaffSignupItemUpdate) signupitems.UpdateInput
	// The new order of the Sign-up items, by id.
	SignupItemOrder(source []staffapi.EntityId) []int64

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// goverter:enum:transform value
	SignupItemKind(source staffapi.SignupItemKind) signupitem.Kind
}

func boolIn(o staffapi.OptBool) bool                   { return o.Or(false) }
func boolPtrIn(o staffapi.OptBool) *bool               { return convert.Ptr(o) }
func titlePtrIn(o staffapi.OptSignupItemTitle) *string { return convert.StringPtr(o) }
