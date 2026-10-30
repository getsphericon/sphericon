// Package resources holds the goverter-generated mapper from ent rows and core module values to
// the core Staff API (gen/staff) resources. The implementation (ConverterImpl) lives in
// converter_gen.go; internal/api/staff instantiates it.
package resources

import (
	"time"

	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/signupitems"
)

// Converter maps ent rows and module values to the core Staff API resources.
//
// goverter:converter
// goverter:name ConverterImpl
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum no
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optTimestamp
// goverter:extend position
// goverter:extend optDraft
// goverter:extend optVersion
type Converter interface {
	StaffMemberToResource(source *ent.StaffMember) staffapi.StaffResource

	SignupItemToResource(source signupitems.Item) staffapi.StaffSignupItemResource
	SignupItemsToResources(source []signupitems.Item) []staffapi.StaffSignupItemResource
	VersionToResource(source signupitems.Version) staffapi.StaffLegalDocumentVersion
	VersionsToResources(source []signupitems.Version) []staffapi.StaffLegalDocumentVersion
}

func timestamp(t time.Time) staffapi.Timestamp { return staffapi.Timestamp(t) }

func optTimestamp(t *time.Time) staffapi.OptTimestamp {
	if t == nil {
		return staffapi.OptTimestamp{}
	}
	return staffapi.NewOptTimestamp(staffapi.Timestamp(*t))
}

func position(p int) int32 { return narrow.Int32(p) }

func optDraft(d *string) staffapi.OptLegalDocumentMarkdown {
	if d == nil {
		return staffapi.OptLegalDocumentMarkdown{}
	}
	return staffapi.NewOptLegalDocumentMarkdown(staffapi.LegalDocumentMarkdown(*d))
}

func optVersion(c Converter, v *signupitems.Version) staffapi.OptStaffLegalDocumentVersion {
	if v == nil {
		return staffapi.OptStaffLegalDocumentVersion{}
	}
	return staffapi.NewOptStaffLegalDocumentVersion(c.VersionToResource(*v))
}
