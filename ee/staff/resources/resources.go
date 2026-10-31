// Package resources holds goverter-generated mappers from ent rows and the platform read models
// to the Enterprise Staff API (gen/staffee) and service API (gen/service) resources, plus the custom
// conversions goverter cannot infer on its own. The implementations live in
// staff_converter_gen.go and service_converter_gen.go; ee/staff instantiates them.
package resources

import (
	"time"

	"github.com/go-faster/jx"

	"github.com/getsphericon/sphericon/ee/abuse"
	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	"github.com/getsphericon/sphericon/ent/workspace"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/reputation"
)

// PeriodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const PeriodLayout = "2006-01"

// StaffConverter maps ent rows and the console's read models to the Enterprise Staff API
// resources.
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
// goverter:extend nilFloat64
// goverter:extend count
// goverter:extend optNilSource
type StaffConverter interface {
	// The console's view of a Workspace: metadata and suspension state only. The Staff
	// member sees the real actor id, which a customer never does.
	// goverter:map PublicID ID
	// goverter:map AbuseExempt Exempt
	// goverter:map AbuseWarningNotifiedAt Warned | isSet
	// goverter:map . Suspension | workspaceSuspension
	WorkspaceToResource(source *ent.Workspace) staffeeapi.StaffWorkspaceResource

	// The detail page is the metadata of WorkspaceToResource plus figures read through other
	// modules (the active holds, the abuse state, the deliverability counts): the handler
	// attaches them, each through its own converter method.
	// goverter:map PublicID ID
	// goverter:map AbuseExempt Exempt
	// goverter:map AbuseWarningNotifiedAt Warned | isSet
	// goverter:map . Suspension | workspaceSuspension
	// goverter:ignore Holds
	// goverter:ignore Abuse
	// goverter:ignore Deliverability
	WorkspaceDetailToResource(source *ent.Workspace) staffeeapi.StaffWorkspaceDetailResource

	HoldToResource(source *ent.Hold) staffeeapi.StaffHold

	// The detected breach, if any, is attached by the handler.
	// goverter:map AbuseExempt Exempt
	// goverter:map AbuseWarningNotifiedAt WarningNotifiedAt
	// goverter:map AbuseFreezeNotifiedAt FreezeNotifiedAt
	// goverter:ignore Warning
	AbuseToResource(source *ent.Workspace) staffeeapi.StaffAbuse
	// goverter:map . Rate | breachRate
	BreachToWarning(source abuse.Breach) staffeeapi.StaffAbuseWarning

	// goverter:map Complaint ComplaintRate
	// goverter:map Bounce BounceRate
	DomainRatesToResource(source abuse.DomainRates) staffeeapi.StaffDomainRates
	RateToResource(source reputation.Rate) staffeeapi.StaffRate
	// goverter:map Window WindowHours | windowHours
	DeliverabilityToResource(source abuse.Deliverability) staffeeapi.StaffDeliverability

	// goverter:map . Actor
	// goverter:map . Target
	AuditEntryToResource(source auditapi.View) staffeeapi.StaffAuditEntryResource
	// An entry's actor and target are columns of the entry, nested in the contract.
	// goverter:map ActorKind Kind
	// goverter:map ActorID ID
	// goverter:map ActorName Name
	AuditActorToResource(source auditapi.View) staffeeapi.StaffAuditActor
	// goverter:map TargetType Type
	// goverter:map TargetID ID
	// goverter:map TargetName Name
	AuditTargetToResource(source auditapi.View) staffeeapi.StaffAuditTarget

	// One User's Sign-up survey answers. The enums share the contract's values.
	// goverter:map UserID UserId
	// goverter:map CreatedAt AnsweredAt
	SignupSurveyResponseToResource(source *ent.SignupSurveyResponse) staffeeapi.StaffSignupSurveyResponseResource
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
// goverter:extend serviceOptString
// goverter:extend serviceOptEmail
// goverter:extend serviceOptTimestamp
// goverter:extend serviceOptNilTimestampAt
type ServiceConverter interface {
	HoldToResource(source *ent.Hold) serviceapi.HoldResource

	// The owners are the feed entry's, which the mapper cannot see: the handler attaches them.
	// goverter:map PublicID ID
	// goverter:ignore Owners
	WorkspaceToResource(source *ent.Workspace) serviceapi.ServiceWorkspaceResource
	// goverter:map ID UserId
	OwnerToResource(source *ent.User) serviceapi.ServiceWorkspaceOwner

	// The mailings consent is the feed entry's, which the mapper cannot see: the handler
	// attaches it.
	// goverter:ignore DeletedAt
	// goverter:ignore MailingsConsent
	// goverter:ignore MailingsConsentChangedAt
	UserToResource(source *ent.User) serviceapi.ServiceUserResource
	// A tombstone carries the deleted User's id and when they were deleted, nothing else.
	// goverter:map UserID ID
	// goverter:map CreatedAt DeletedAt
	// goverter:ignore Name
	// goverter:ignore Email
	// goverter:ignore EmailVerifiedAt
	// goverter:ignore CreatedAt
	// goverter:ignore MailingsConsent
	// goverter:ignore MailingsConsentChangedAt
	TombstoneToResource(source *ent.UserTombstone) serviceapi.ServiceUserResource

	UsageFigureToResource(source usage.Figure) serviceapi.UsageFigure
	// The public id is the Workspace's, which the usage row does not carry: the handler
	// attaches it.
	// goverter:ignore WorkspaceId
	// goverter:map Period Period | periodString
	UsageToResource(source usage.WorkspaceUsage) serviceapi.WorkspaceUsageResource
}

func staffTimestamp(t time.Time) staffeeapi.Timestamp { return staffeeapi.Timestamp(t) }

func serviceTimestamp(t time.Time) serviceapi.Timestamp { return serviceapi.Timestamp(t) }

func nilableString(v *string) staffeeapi.OptNilString {
	return convert.Opt[staffeeapi.OptNilString](v)
}

func serviceNilableString(v *string) serviceapi.OptNilString {
	return convert.Opt[serviceapi.OptNilString](v)
}

func optNilTimestamp(t *time.Time) staffeeapi.OptNilTimestamp {
	return convert.Opt[staffeeapi.OptNilTimestamp]((*staffeeapi.Timestamp)(t))
}

func serviceOptNilTimestamp(t *time.Time) serviceapi.OptNilTimestamp {
	return convert.Opt[serviceapi.OptNilTimestamp]((*serviceapi.Timestamp)(t))
}

func serviceOptString(v string) serviceapi.OptString { return serviceapi.NewOptString(v) }

func serviceOptEmail(v string) serviceapi.OptEmailAddress {
	return serviceapi.NewOptEmailAddress(serviceapi.EmailAddress(v))
}

func serviceOptTimestamp(t time.Time) serviceapi.OptTimestamp {
	return serviceapi.NewOptTimestamp(serviceapi.Timestamp(t))
}

func serviceOptNilTimestampAt(t time.Time) serviceapi.OptNilTimestamp {
	return serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(t))
}

func isSet(t *time.Time) bool { return t != nil }

func breachRate(b abuse.Breach) float64 { return b.Rate() }

func periodString(t time.Time) string { return t.Format(PeriodLayout) }

// workspaceSuspension is the Workspace's suspension and who set it; absent unless it is suspended.
func workspaceSuspension(w *ent.Workspace) staffeeapi.OptNilStaffSuspension {
	if w.SuspendedAt == nil {
		return staffeeapi.OptNilStaffSuspension{}
	}
	actor := staffeeapi.StaffSuspensionActor{}
	if w.SuspendedByKind != nil {
		actor.Kind = staffeeapi.StaffSuspensionActorKind(*w.SuspendedByKind)
	}
	if w.SuspendedByKind != nil && *w.SuspendedByKind == workspace.SuspendedByKindStaff && w.SuspendedByID != nil {
		actor.ID = staffeeapi.NewOptNilString(*w.SuspendedByID)
	}
	return staffeeapi.NewOptNilStaffSuspension(staffeeapi.StaffSuspension{
		At:     staffeeapi.Timestamp(*w.SuspendedAt),
		Actor:  actor,
		Reason: nilableString(w.SuspensionReason),
	})
}

func staffAuditDiff(d map[string]jx.Raw) staffeeapi.OptNilStaffAuditEntryResourceDiff {
	return convert.OptObject[staffeeapi.OptNilStaffAuditEntryResourceDiff](staffeeapi.StaffAuditEntryResourceDiff(d))
}

func nilFloat64(v *float64) staffeeapi.NilFloat64 {
	return convert.Nil[staffeeapi.NilFloat64](v)
}

// count narrows a count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

func optNilSource(v *signupsurveyresponse.HeardFrom) staffeeapi.OptNilStaffSignupSource {
	return convert.Opt[staffeeapi.OptNilStaffSignupSource]((*staffeeapi.StaffSignupSource)(v))
}

func windowHours(d time.Duration) int32 { return narrow.Int32(d / time.Hour) }
