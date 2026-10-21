// Package resources holds goverter-generated mappers from ent rows and the platform read models
// to the Staff API (gen/staff) and service API (gen/service) resources, plus the custom
// conversions goverter cannot infer on its own. The implementations live in
// staff_converter_gen.go and service_converter_gen.go; ee/staff instantiates them.
package resources

import (
	"time"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/auditapi"
)

// PeriodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const PeriodLayout = "2006-01"

// StaffConverter maps ent rows and the console's read models to the Staff API resources.
//
// goverter:converter
// goverter:name StaffConverterImpl
// goverter:output:file ./staff_converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum no
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend staffTimestamp
// goverter:extend nilableString
// goverter:extend optNilTimestamp
// goverter:extend staffAuditDiff
type StaffConverter interface {
	StaffMemberToResource(source *ent.StaffMember) staffapi.StaffResource

	// The console's view of a Workspace: metadata and suspension state only. The Staff
	// member sees the real actor id, which a customer never does.
	// goverter:map PublicID ID
	// goverter:map AbuseExempt Exempt
	// goverter:map AbuseWarningNotifiedAt Warned | isSet
	// goverter:map . Suspension | workspaceSuspension
	WorkspaceToResource(source *ent.Workspace) staffapi.StaffWorkspaceResource

	HoldToResource(source *ent.Hold) staffapi.StaffHold

	// The detected breach, if any, is attached by the handler.
	// goverter:map AbuseExempt Exempt
	// goverter:map AbuseWarningNotifiedAt WarningNotifiedAt
	// goverter:map AbuseFreezeNotifiedAt FreezeNotifiedAt
	// goverter:ignore Warning
	AbuseToResource(source *ent.Workspace) staffapi.StaffAbuse
	// goverter:map . Rate | breachRate
	BreachToWarning(source abuse.Breach) staffapi.StaffAbuseWarning

	// goverter:map . Actor | staffAuditActor
	// goverter:map . Target | staffAuditTarget
	AuditEntryToResource(source auditapi.View) staffapi.StaffAuditEntryResource
}

// ServiceConverter maps ent rows and the usage figures to the service API resources.
//
// goverter:converter
// goverter:name ServiceConverterImpl
// goverter:output:file ./service_converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum no
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend serviceTimestamp
// goverter:extend serviceNilableString
// goverter:extend serviceOptNilTimestamp
type ServiceConverter interface {
	HoldToResource(source *ent.Hold) serviceapi.HoldResource

	// The owners are the feed entry's, which the mapper cannot see: the handler attaches them.
	// goverter:map PublicID ID
	// goverter:ignore Owners
	WorkspaceToResource(source *ent.Workspace) serviceapi.ServiceWorkspaceResource
	// goverter:map ID UserId
	OwnerToResource(source *ent.User) serviceapi.ServiceWorkspaceOwner

	UsageFigureToResource(source usage.Figure) serviceapi.UsageFigure
	// The public id is the Workspace's, which the usage row does not carry: the handler
	// attaches it.
	// goverter:ignore WorkspaceId
	// goverter:map Period Period | periodString
	UsageToResource(source usage.WorkspaceUsage) serviceapi.WorkspaceUsageResource
}

func staffTimestamp(t time.Time) staffapi.Timestamp { return staffapi.Timestamp(t) }

func serviceTimestamp(t time.Time) serviceapi.Timestamp { return serviceapi.Timestamp(t) }

func nilableString(v *string) staffapi.OptNilString {
	if v == nil {
		return staffapi.OptNilString{}
	}
	return staffapi.NewOptNilString(*v)
}

func serviceNilableString(v *string) serviceapi.OptNilString {
	if v == nil {
		return serviceapi.OptNilString{}
	}
	return serviceapi.NewOptNilString(*v)
}

func optNilTimestamp(t *time.Time) staffapi.OptNilTimestamp {
	if t == nil {
		return staffapi.OptNilTimestamp{}
	}
	return staffapi.NewOptNilTimestamp(staffapi.Timestamp(*t))
}

func serviceOptNilTimestamp(t *time.Time) serviceapi.OptNilTimestamp {
	if t == nil {
		return serviceapi.OptNilTimestamp{}
	}
	return serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(*t))
}

func isSet(t *time.Time) bool { return t != nil }

func breachRate(b abuse.Breach) float64 { return b.Rate() }

func periodString(t time.Time) string { return t.Format(PeriodLayout) }

// workspaceSuspension is the Workspace's suspension and who set it; absent unless it is suspended.
func workspaceSuspension(w ent.Workspace) staffapi.OptNilStaffSuspension {
	if w.SuspendedAt == nil {
		return staffapi.OptNilStaffSuspension{}
	}
	actor := staffapi.StaffSuspensionActor{}
	if w.SuspendedByKind != nil {
		actor.Kind = staffapi.StaffSuspensionActorKind(*w.SuspendedByKind)
	}
	if w.SuspendedByKind != nil && *w.SuspendedByKind == workspace.SuspendedByKindStaff && w.SuspendedByID != nil {
		actor.ID = staffapi.NewOptNilString(*w.SuspendedByID)
	}
	return staffapi.NewOptNilStaffSuspension(staffapi.StaffSuspension{
		At:     staffapi.Timestamp(*w.SuspendedAt),
		Actor:  actor,
		Reason: nilableString(w.SuspensionReason),
	})
}

func staffAuditActor(v auditapi.View) staffapi.StaffAuditActor {
	return staffapi.StaffAuditActor{Kind: staffapi.StaffAuditActorKind(v.ActorKind), ID: nilableString(v.ActorID), Name: nilableString(v.ActorName)}
}

func staffAuditTarget(v auditapi.View) staffapi.StaffAuditTarget {
	return staffapi.StaffAuditTarget{Type: v.TargetType, ID: nilableString(v.TargetID), Name: nilableString(v.TargetName)}
}

func staffAuditDiff(d map[string]jx.Raw) staffapi.OptNilStaffAuditEntryResourceDiff {
	if d == nil {
		return staffapi.OptNilStaffAuditEntryResourceDiff{}
	}
	return staffapi.NewOptNilStaffAuditEntryResourceDiff(d)
}
