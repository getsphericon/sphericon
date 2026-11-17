package resources

import (
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/abusedetector"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
)

// StaffInputs maps the Enterprise Staff API's requests onto the module's inputs: the input
// direction of the contract, so a handler only hands the module what the request says. The acting
// Staff member comes from the session, never the request: each method ignores it and the handler
// assigns it after the call.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name StaffInputsImpl
// goverter:output:file ./staff_inputs_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend staffStringIn
// goverter:extend staffIntIn
// goverter:extend staffReviewIn
// goverter:extend staffMaxPerDayIn
type StaffInputs interface {
	// goverter:map Search | staffSearchIn
	WorkspacesList(source staffeeapi.StaffWorkspacesListParams) workspaceoversight.ListWorkspacesInput
	AuditEntries(source staffeeapi.StaffWorkspaceAuditListParams) workspaceoversight.AuditEntriesInput

	// goverter:ignore By
	SendCapSet(source *staffeeapi.StaffSendCapInput) workspaceoversight.SetSendCapInput
	// Clearing the Send cap is no cap at all: PerDay stays nil.
	// goverter:ignore By PerDay
	SendCapClear(source staffeeapi.StaffWorkspacesClearSendCapParams) workspaceoversight.SetSendCapInput

	// goverter:ignore By
	WorkspaceSuspend(source *staffeeapi.StaffSuspendInput) workspaceoversight.SuspendWorkspaceInput

	// Whether the Workspace is exempt after the change is the operation's, which the handler
	// assigns.
	// goverter:ignore By Exempt
	AbuseExempt(source staffeeapi.StaffWorkspacesExemptParams) workspaceoversight.SetAbuseExemptInput
	// goverter:ignore By Exempt
	AbuseUnexempt(source staffeeapi.StaffWorkspacesUnexemptParams) workspaceoversight.SetAbuseExemptInput

	// The review state a list is filtered by, the one enum the console sends.
	// goverter:enum:transform value
	// goverter:enum:map StaffWorkspaceReviewSystemFrozen ReviewSystemFrozen
	ReviewIn(source staffeeapi.StaffWorkspaceReview) abusedetector.Review
}

func staffStringIn(o staffeeapi.OptString) string { return o.Or("") }
func staffIntIn(o staffeeapi.OptInt32) int        { return int(o.Value) }
func staffSearchIn(o staffeeapi.OptString) string { return strings.TrimSpace(o.Value) }
func staffMaxPerDayIn(v staffeeapi.MaxPerDay) *int {
	perDay := int(v)
	return &perDay
}

// staffReviewIn is the review state a list is filtered by; an absent one is none, which narrows
// the list to no state.
func staffReviewIn(c StaffInputs, o staffeeapi.OptStaffWorkspaceReview) abusedetector.Review {
	v, ok := o.Get()
	if !ok {
		return abusedetector.ReviewNone
	}
	return c.ReviewIn(v)
}

// ServiceInputs maps the service API's requests onto the module's inputs: the input direction of
// the contract, so a handler only hands the module what the request says. The extends below bind the
// generated option types of this surface to the shared conversions of internal/kernel/convert. The
// Service credential comes from the request's authentication, never its body: each method ignores
// it and the handler assigns it after the call.
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
	// The hold's key is the path's, which the handler assigns.
	// goverter:map . Hold
	// goverter:ignore Credential Key
	SetHold(source *serviceapi.HoldInput) workspaceoversight.SetHoldInput
	// goverter:ignore Credential
	ClearHold(source serviceapi.ServiceHoldsClearParams) workspaceoversight.ClearHoldInput
}

func serviceTimestampIn(t serviceapi.Timestamp) time.Time     { return time.Time(t) }
func serviceNilStringPtrIn(o serviceapi.OptNilString) *string { return convert.Ptr(o) }
