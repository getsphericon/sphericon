package resources

import (
	"time"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/holds"
)

// ServiceInputs maps the service API's request bodies onto the modules' inputs: the input direction
// of the contract, so a handler only hands the module what the request says. The extends below bind
// the generated option types of this surface to the shared conversions of internal/convert. The
// Enterprise Staff API (gen/staffee) has no body that is a module's input: its operations pass the
// module single values.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name ServiceInputsImpl
// goverter:output:file ./service_inputs_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend serviceTimestampIn
// goverter:extend serviceNilStringPtrIn
type ServiceInputs interface {
	HoldSet(source *serviceapi.HoldInput) holds.Input
}

func serviceTimestampIn(t serviceapi.Timestamp) time.Time     { return time.Time(t) }
func serviceNilStringPtrIn(o serviceapi.OptNilString) *string { return convert.Ptr(o) }
