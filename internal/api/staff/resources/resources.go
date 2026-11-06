// Package resources holds the goverter-generated mapper from ent rows and core module values to
// the core Staff API (gen/staff) resources. The implementation (ConverterImpl) lives in
// converter_gen.go; internal/api/staff instantiates it.
package resources

import (
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupitem"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/instancestates"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
)

// Converter maps ent rows and module values to the core Staff API resources.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name ConverterImpl
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optTimestamp
// goverter:extend position
// goverter:extend optDraft
// goverter:extend optVersion
// goverter:extend optString
// goverter:extend optLapse
// goverter:extend optLock
// goverter:extend optDefaultSendCap
// goverter:extend nilMaxPerDay
// goverter:extend instanceAuditDiff
type Converter interface {
	StaffMemberToResource(source *ent.StaffMember) staffapi.StaffResource

	InstanceStateToResource(source instancestates.State) staffapi.StaffInstanceState
	DefaultSendCapToResource(source instancestates.DefaultSendCap) staffapi.StaffDefaultSendCap
	DefaultSendCapChangeToResource(source instancestates.DefaultSendCapChange) staffapi.StaffDefaultSendCapChange

	// An entry's actor is the Staff member it was loaded with; it is never deleted (ADR 0026).
	// goverter:map CreatedAt OccurredAt
	// goverter:map Edges.StaffMember Actor
	InstanceAuditEntryToResource(source *ent.InstanceAuditEntry) staffapi.StaffInstanceAuditEntryResource
	// goverter:useEmptyOnNil:slice
	InstanceAuditEntriesToResources(source []*ent.InstanceAuditEntry) []staffapi.StaffInstanceAuditEntryResource

	SignupItemToResource(source signupitems.Item) staffapi.StaffSignupItemResource
	SignupItemsToResources(source []signupitems.Item) []staffapi.StaffSignupItemResource
	VersionToResource(source signupitems.Version) staffapi.StaffLegalDocumentVersion
	VersionsToResources(source []signupitems.Version) []staffapi.StaffLegalDocumentVersion

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The Staff member deciding is no lock: optLock leaves it out.
	// goverter:enum:transform value
	// goverter:enum:map LockNone @panic
	InstanceTelemetryLockToResource(source instancestates.Lock) staffapi.StaffInstanceTelemetryLock
	// goverter:enum:transform value
	SignupItemKindToResource(source signupitem.Kind) staffapi.SignupItemKind
}

func timestamp(t time.Time) staffapi.Timestamp { return staffapi.Timestamp(t) }

func optTimestamp(t *time.Time) staffapi.OptTimestamp {
	return convert.Opt[staffapi.OptTimestamp]((*staffapi.Timestamp)(t))
}

func position(p int) int32 { return narrow.Int32(p) }

func optString(v *string) staffapi.OptString {
	return convert.Opt[staffapi.OptString](v)
}

// optLapse is when the license key expired, absent unless it has lapsed.
func optLapse(t time.Time) staffapi.OptTimestamp {
	if t.IsZero() {
		return staffapi.OptTimestamp{}
	}
	return staffapi.NewOptTimestamp(staffapi.Timestamp(t))
}

// optLock is what decides the Telemetry report, absent while the Staff member does.
func optLock(c Converter, l instancestates.Lock) staffapi.OptStaffInstanceTelemetryLock {
	if l == instancestates.LockNone {
		return staffapi.OptStaffInstanceTelemetryLock{}
	}
	return staffapi.NewOptStaffInstanceTelemetryLock(c.InstanceTelemetryLockToResource(l))
}

// optDefaultSendCap is the default Send cap, absent on an Instance that has no Send caps.
func optDefaultSendCap(c Converter, d *instancestates.DefaultSendCap) staffapi.OptStaffDefaultSendCap {
	if d == nil {
		return staffapi.OptStaffDefaultSendCap{}
	}
	return staffapi.NewOptStaffDefaultSendCap(c.DefaultSendCapToResource(*d))
}

// nilMaxPerDay renders a Send cap: null when there is none.
func nilMaxPerDay(v *int) staffapi.NilMaxPerDay {
	if v == nil {
		return staffapi.NilMaxPerDay{Null: true}
	}
	return staffapi.NewNilMaxPerDay(staffapi.MaxPerDay(narrow.Int32(*v)))
}

// instanceAuditDiff is an entry's stored diff as the contract's object.
func instanceAuditDiff(d map[string]any) staffapi.JsonObject {
	return convert.RawObject[staffapi.JsonObject](d)
}

func optDraft(d *string) staffapi.OptLegalDocumentMarkdown {
	return convert.Opt[staffapi.OptLegalDocumentMarkdown]((*staffapi.LegalDocumentMarkdown)(d))
}

func optVersion(c Converter, v *signupitems.Version) staffapi.OptStaffLegalDocumentVersion {
	if v == nil {
		return staffapi.OptStaffLegalDocumentVersion{}
	}
	return staffapi.NewOptStaffLegalDocumentVersion(c.VersionToResource(*v))
}
