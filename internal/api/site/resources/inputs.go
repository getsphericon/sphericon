package resources

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	"github.com/getsphericon/sphericon/ent/workspace"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/content/emailtemplates"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/invitations"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/passkeys"
	"github.com/getsphericon/sphericon/internal/domain/identity/secondfactors"
	"github.com/getsphericon/sphericon/internal/domain/identity/signup"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupsurveyresponses"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/insights/dataexport"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/kernel/tokenscope"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// Inputs maps request bodies onto the modules' inputs: the input direction of the contract, so a
// handler only hands the module what the request says. An update becomes the module's patch
// (internal/kernel/patch): an absent key keeps, an explicit null clears, a value sets. The extends below
// bind the generated option types of this surface to the shared conversions of internal/kernel/convert.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name InputsImpl
// goverter:output:file ./inputs_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend stringPatch
// goverter:extend revisionIn
// goverter:extend namePatch
// goverter:extend dateFieldIn
// goverter:extend dateOffsetDaysIn
// goverter:extend nameIn
// goverter:extend namePtrIn
// goverter:extend nilStringPatch
// goverter:extend nilEmailPatch
// goverter:extend nilTimeZonePatch
// goverter:extend nilCountryPatch
// goverter:extend nilLocalePatch
// goverter:extend nilImageURLPatch
// goverter:extend nilDatePatch
// goverter:extend nilEntityIDPatch
// goverter:extend boolPatch
// goverter:extend maxPerSecondPatch
// goverter:extend maxPerDayPatch
// goverter:extend listPatch
// goverter:extend buttonsPtrIn
// goverter:extend buttonsPatch
// goverter:extend stringIn
// goverter:extend emailIn
// goverter:extend signingSecretIn
// goverter:extend stringPtrIn
// goverter:extend emailPtrIn
// goverter:extend entityIDPtrIn
// goverter:extend nilStringPtrIn
// goverter:extend nilEmailPtrIn
// goverter:extend nilTimeZonePtrIn
// goverter:extend nilCountryPtrIn
// goverter:extend nilLocalePtrIn
// goverter:extend nilImageURLPtrIn
// goverter:extend nilDatePtrIn
// goverter:extend nilEntityIDPtrIn
// goverter:extend requiredNilEntityIDPtrIn
// goverter:extend boolIn
// goverter:extend boolPtrIn
// goverter:extend entityIDIn
// goverter:extend maxPerSecondPtrIn
// goverter:extend maxPerDayPtrIn
// goverter:extend currencyCodePtrIn
// goverter:extend attributionWindowDaysPtrIn
// goverter:extend integrationConfigIn
// goverter:extend integrationConfigPatch
// goverter:extend int32ToInt
// goverter:extend listPtrIn
// goverter:extend timestampPtrIn
// goverter:extend automationPathPtrIn
// goverter:extend automationStepsIn
// goverter:extend stepIDIn
// goverter:extend ruleActionsPtrIn
// goverter:extend documentIn
// goverter:extend documentPatch
// goverter:extend objectIn
// goverter:extend objectPatch
// goverter:extend nilObjectIn
// goverter:extend nilObjectPatch
// goverter:extend signupSourcePtrIn
// goverter:extend customFieldTypePatch
// goverter:extend broadcastContentIn
// goverter:extend broadcastContentPatch
// goverter:extend broadcastChannelPtrIn
// goverter:extend automationStateIn
// goverter:extend enrollmentStatusIn
// goverter:extend automationsSortIn
// goverter:extend sortDirectionIn
// goverter:extend timeIn
// goverter:extend branchConditionIn
type Inputs interface {
	// The proof of identity comes from the session and the password typed, which the handler
	// assigns.
	// goverter:ignore Proof
	SecondFactorConfirm(source *siteapi.SiteSecondFactorConfirmInput) secondfactors.ConfirmInput
	// The proof of identity comes from the session and the password typed, which the handler
	// assigns.
	// goverter:ignore Proof
	SecondFactorDisable(source *siteapi.SiteSecondFactorDisableInput) secondfactors.DisableInput
	// The Passkey is the path's, which the handler assigns.
	// goverter:ignore ID
	PasskeyRename(source *siteapi.SitePasskeyRenameInput) passkeys.RenameInput
	// The proof of identity comes from the session and the password typed, which the handler
	// assigns.
	// goverter:ignore Proof
	PasskeyRemove(source siteapi.SitePasskeysRemoveParams) passkeys.RemoveInput
	// The proof of identity comes from the session and the password typed, which the handler
	// assigns.
	// goverter:ignore Proof
	IdentityLinkStart(source *siteapi.SiteIdentityLinkInput) signup.LinkStartInput

	// The restoring User, the actor the Audit log records and whether the session is strong come
	// from the session, which the handler assigns.
	// goverter:map ID PublicID
	// goverter:ignore UserID Actor StrongSession
	WorkspaceRestore(source siteapi.SiteWorkspacesRestoreParams) workspaceview.RestoreInput
	// The creating User and whether the session is strong come from the session, which the
	// handler assigns.
	// goverter:ignore UserID StrongSession
	WorkspaceCreate(source *siteapi.SiteCreateWorkspaceInput) workspaceview.CreateInput
	// An absent postal address leaves it unchanged; a present one, empty included, sets it.
	WorkspaceUpdate(source *siteapi.SiteUpdateWorkspaceInput) workspaceview.UpdateInput
	// goverter:map Timezone Zone
	TimezoneSet(source *siteapi.SiteWorkspaceTimezoneInput) workspaceview.SetTimezoneInput
	// goverter:map EngagementTrackingDefault Default
	EngagementTrackingDefaultSet(source *siteapi.SiteSetEngagementTrackingDefaultInput) workspaceview.SetEngagementTrackingDefaultInput
	// ent's default constant shares track's value, so each value is named.
	// goverter:enum:map EngagementTrackingDecisionTrack EngagementTrackingDefaultTrack
	// goverter:enum:map EngagementTrackingDecisionDontTrack EngagementTrackingDefaultDontTrack
	EngagementTrackingDefaultIn(source siteapi.EngagementTrackingDecision) workspace.EngagementTrackingDefault
	RevenueSettingsSet(source *siteapi.SiteUpdateRevenueSettingsInput) workspaces.SetRevenueSettingsInput
	ConfirmationEmailSet(source *siteapi.SiteUpdateConfirmationEmailInput) workspaces.SetConfirmationEmailInput
	// goverter:map Enabled On
	ConfirmedOptInSet(source *siteapi.SiteSetConfirmedOptInInput) workspaceview.SetConfirmedOptInInput
	// Switching the requirement on: the caller sets Required.
	// goverter:map Revision | revisionPtrIn
	// goverter:ignore Required
	SecondFactorRequire(source *siteapi.RevisionInput) workspaceview.SetSecondFactorRequirementInput
	// The Sign-up browser is the request's cookie, a parameter the handler assigns.
	// goverter:ignore Browser
	VerifyEmailLink(source *siteapi.SiteVerifyEmailInput) signup.VerifyEmailLinkInput
	EmailChange(source *siteapi.SiteEmailChangeInput) users.EmailChangeInput
	// The flow and the session holder are the request's cookies, which the handler opens and
	// assigns.
	// goverter:ignore Flow Session
	SigninCallback(source siteapi.SiteAuthSigninCallbackParams) signup.ProviderSignInInput

	TemplateCreate(source *siteapi.CreateTemplateInput) emailtemplates.CreateInput
	TemplateUpdate(source *siteapi.UpdateTemplateInput) emailtemplates.UpdateInput

	ConsentEvidence(source *siteapi.ConsentEvidence) confirmations.Evidence
	// A member's Engagement tracking observation of one of a Contact's Destinations (ADR 0120).
	EngagementTrackingByMember(source *siteapi.SiteRecordEngagementTrackingInput) engagementtracking.MemberInput
	// goverter:enum:transform value
	EngagementTrackingChannel(source siteapi.EngagementTrackingChannel) channels.Channel
	// goverter:enum:transform value
	EngagementTrackingState(source siteapi.EngagementTrackingState) engagementtrackingconsent.State
	// Where a Broadcast's Test send goes: the address on email, the number on sms. OnlyTo is the
	// handler's to set.
	// goverter:ignore OnlyTo
	TestAddress(source *siteapi.TestSendBroadcastInput) outboundmessages.TestAddress

	BroadcastCreate(source *siteapi.CreateBroadcastInput) (broadcasts.CreateInput, error)
	BroadcastUpdate(source *siteapi.UpdateBroadcastInput) (broadcasts.UpdateInput, error)
	// The arms of a Broadcast's content, which broadcastContentIn and broadcastContentPatch pick.
	EmailBroadcastContentIn(source siteapi.EmailBroadcastContentInput) broadcasts.EmailContentInput
	EmailBroadcastContentPatch(source siteapi.EmailBroadcastContentInput) broadcasts.EmailContentPatch
	TelegramBroadcastContentIn(source siteapi.TelegramBroadcastContentInput) broadcasts.TelegramContentInput
	TelegramBroadcastContentPatch(source siteapi.TelegramBroadcastContentInput) broadcasts.TelegramContentPatch
	SmsBroadcastContentIn(source siteapi.SmsBroadcastContentInput) broadcasts.SMSContentInput
	SmsBroadcastContentPatch(source siteapi.SmsBroadcastContentInput) broadcasts.SMSContentPatch
	// The list's filter; the statuses go member by member (BroadcastStatuses). The order, which has a
	// default of its own, is BroadcastsOrder's.
	// goverter:map Status Statuses
	// goverter:ignore Order
	BroadcastsListFilter(source siteapi.SiteBroadcastsListParams) broadcasts.ListFilter
	BroadcastSchedule(source *siteapi.ScheduleBroadcastInput) broadcasts.ScheduleInput
	AutomationsListFilter(source siteapi.SiteAutomationsListParams) automations.ListFilter
	// goverter:map AutomationId AutomationID
	EnrollmentsFilter(source siteapi.SiteEnrollmentsListParams) automations.EnrollmentsFilter
	// goverter:map AutomationId AutomationID
	// goverter:map ContactId ContactID | entityIDIn
	SkippedEntriesFilter(source siteapi.SiteSkippedEntriesListParams) automations.SkippedEntriesFilter
	// The Contact a Data export names by its id or its email.
	ContactExport(source siteapi.SiteContactsExportParams) dataexport.OpenInput

	WebhookCreate(source *siteapi.CreateWebhookInput) webhookendpoints.CreateInput
	WebhookUpdate(source *siteapi.UpdateWebhookInput) webhookendpoints.UpdateInput

	// goverter:map DkimSelector Selector
	SendingDomainCreate(source *siteapi.CreateSendingDomainInput) sendingdomains.CreateInput

	// goverter:map . Attributes
	ContactCreate(source *siteapi.SiteCreateContactInput) contacts.CreateInput
	ContactAttributes(source siteapi.SiteCreateContactInput) contacts.Attributes
	// goverter:map . Core
	ContactUpdate(source *siteapi.SiteUpdateContactInput) contacts.UpdateInput

	CustomFieldUpdate(source *siteapi.UpdateCustomFieldInput) contacts.CustomFieldUpdateInput

	IntegrationCreate(source *siteapi.CreateIntegrationInput) (integrations.CreateInput, error)
	IntegrationUpdate(source *siteapi.UpdateIntegrationInput) (integrations.UpdateInput, error)

	// goverter:map Condition | createSegmentCondition
	SegmentCreate(source *siteapi.CreateSegmentInput) (segments.CreateInput, error)
	// goverter:map Condition | segmentConditionPatch
	SegmentUpdate(source *siteapi.UpdateSegmentInput) (segments.UpdateInput, error)
	// goverter:map Condition | segmentConditionIn
	SegmentPreview(source *siteapi.SitePreviewSegmentInput) (segments.PreviewInput, error)

	// A Step of a fork's path as a Step of the root's union: the two unions have the same arms, so
	// an arm one of them lacks fails generation (pathStepsIn).
	// goverter:skipCopySameType
	PathStepInputSum(source siteapi.PathStepInputSum) siteapi.AutomationStepInputSum
	// goverter:enum:transform value
	PathStepInputType(source siteapi.PathStepInputSumType) siteapi.AutomationStepInputSumType
	// A Step of a Split's path as a Step of the root's union, as PathStepInputSum (splitPathStepsIn).
	// goverter:skipCopySameType
	SplitPathStepInputSum(source siteapi.SplitPathStepInputSum) siteapi.AutomationStepInputSum
	// goverter:enum:transform value
	SplitPathStepInputType(source siteapi.SplitPathStepInputSumType) siteapi.AutomationStepInputSumType
	// The Conditions of a Branch being edited, whose need of an unknown path the editor asks.
	BranchConditions(source []siteapi.BranchConditionInput) ([]conditiontree.Branch, error)
	// A version's root path; each Step is picked by automationStepIn.
	AutomationPath(source siteapi.AutomationPathInput) (automationstep.Path, error)
	// The arms of a Step, which automationStepIn picks; the Step's id is the arm's, which it
	// takes. The editor version is the save's own: the module renders the document and stamps it.
	// goverter:ignore EditorVersion
	EmailStepIn(source siteapi.EmailStepInput) automationstep.Email
	TelegramStepIn(source siteapi.TelegramStepInput) automationstep.Telegram
	SmsStepIn(source siteapi.SmsStepInput) automationstep.SMS
	WaitStepIn(source siteapi.WaitStepInput) automationstep.Wait
	// goverter:map Until | waitUntilMomentIn
	WaitUntilStepIn(source siteapi.WaitUntilStepInput) (automationstep.WaitUntil, error)
	WaitUntilTimeOfDayIn(source siteapi.WaitUntilTimeOfDayInput) automationstep.TimeOfDay
	// goverter:enum:transform value
	WeekdayIn(source siteapi.Weekday) localcalendar.Weekday
	ApplyTagStepIn(source siteapi.ApplyTagStepInput) automationstep.ApplyTag
	RemoveTagStepIn(source siteapi.RemoveTagStepInput) automationstep.RemoveTag
	HttpStepIn(source siteapi.HttpStepInput) automationstep.HTTP
	TelegramButton(source siteapi.TelegramButtonInput) telegramrender.Button
	// goverter:map Trigger | automationTriggerIn
	// goverter:map Trigger Entry | automationEntryIn
	// goverter:map ExitEvents | automationExitsIn
	// goverter:map GoalEvent | automationGoalIn
	AutomationCreate(source *siteapi.CreateAutomationInput) (automations.CreateInput, error)
	// goverter:map Trigger | automationTriggerPtrIn
	// goverter:map Trigger Entry | automationEntryPtrIn
	// goverter:map ExitEvents | automationExitsPtrIn
	// goverter:map GoalEvent | automationGoalPatch
	AutomationUpdate(source *siteapi.UpdateAutomationInput) (automations.UpdateInput, error)

	RuleActions(source []siteapi.RuleActionInput) []ruleaction.Action
	// goverter:map Condition | ruleConditionIn
	RuleCreate(source *siteapi.CreateRuleInput) (rules.CreateInput, error)
	// goverter:map Condition | ruleConditionIn
	RuleUpdate(source *siteapi.UpdateRuleInput) (rules.UpdateInput, error)

	TokenCreate(source *siteapi.SiteCreateTokenInput) apitokens.Input
	PersonalTokenCreate(source *siteapi.SiteCreatePersonalTokenInput) apitokens.PersonalInput

	BrandKitSave(source *siteapi.SiteSaveBrandKitInput) brandkits.Input

	SignUp(source *siteapi.SiteSignUpInput) signup.SignUpInput
	SignupAnswers(source []siteapi.SiteSignupItemAnswer) []consentrecords.Answer
	ReAcceptances(source []siteapi.SiteAcceptance) []consentrecords.ReAcceptance

	// The inviting User is the session's, not the request's: the handler sets it.
	// goverter:ignore InvitedBy
	InvitationCreate(source *siteapi.SiteCreateInvitationInput) invitations.InviteInput
	// The Membership changed is the path's, not the request's: the handler sets it.
	// goverter:ignore ID
	MembershipRoleChange(source *siteapi.SiteUpdateMembershipInput) memberships.ChangeRoleInput
	// The optional consent changed is the path's key, not the request's: the handler sets it.
	// goverter:ignore Key
	OptionalConsentChange(source *siteapi.SiteOptionalConsentChange) consentrecords.SetOptionalConsentInput

	// Every answer was checked against the contract's enums when the request was decoded.
	SignupSurvey(source *siteapi.SiteSignupSurveyInput) signupsurveyresponses.Answers

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a window "7d", the module counts its days: each member is mapped.
	// goverter:enum:map SiteAnalyticsRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRange90d Window90Days
	AnalyticsWindow(source siteapi.SiteAnalyticsRange) analyticsreport.Window
	// ent's EntryPolicy has a Default constant beside EntryPolicyOnce, so every member is mapped.
	// goverter:enum:map EventTriggerInputEntryPolicyOnce EntryPolicyOnce
	// goverter:enum:map EventTriggerInputEntryPolicyAgain EntryPolicyAgain
	EntryPolicy(source siteapi.EventTriggerInputEntryPolicy) automationversion.EntryPolicy
	// goverter:enum:transform value
	DateRecurrence(source siteapi.DateRecurrence) localcalendar.Recurrence
	// A date Trigger and a Wait until on a date take the same occurrence of a date (ADR 0126).
	DateTriggerIn(source siteapi.DateTriggerInput) datetrigger.Date
	WaitUntilDateIn(source siteapi.WaitUntilDateInput) datetrigger.Date
	// goverter:enum:transform value
	SuppressionChannel(source siteapi.SuppressionChannel) channels.Channel
	// goverter:enum:map SiteAnalyticsRevenueRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRevenueRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRevenueRange90d Window90Days
	AnalyticsRevenueWindow(source siteapi.SiteAnalyticsRevenueRange) analyticsreport.Window
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map SiteAutomationReportsGetPeriod7d ReportPeriod7Days
	// goverter:enum:map SiteAutomationReportsGetPeriod14d ReportPeriod14Days
	// goverter:enum:map SiteAutomationReportsGetPeriod30d ReportPeriod30Days
	// goverter:enum:map SiteAutomationReportsGetPeriod90d ReportPeriod90Days
	AutomationReportPeriod(source siteapi.SiteAutomationReportsGetPeriod) automations.ReportPeriod
	// ent's Status has a Default constant beside StatusDraft, so every member is mapped.
	// goverter:enum:map BroadcastStatusDraft StatusDraft
	// goverter:enum:map BroadcastStatusScheduled StatusScheduled
	// goverter:enum:map BroadcastStatusSending StatusSending
	// goverter:enum:map BroadcastStatusSent StatusSent
	// goverter:enum:map BroadcastStatusFailed StatusFailed
	// goverter:enum:map BroadcastStatusCancelled StatusCancelled
	BroadcastStatus(source siteapi.BroadcastStatus) broadcast.Status
	BroadcastStatuses(source []siteapi.BroadcastStatus) []broadcast.Status
	// goverter:enum:transform value
	BroadcastsOrder(source siteapi.SiteBroadcastsOrder) broadcasts.Order
	// goverter:enum:transform value
	BroadcastChannel(source siteapi.BroadcastChannel) channels.Channel
	// goverter:enum:transform value
	AutomationState(source siteapi.SiteAutomationState) automations.State
	// goverter:enum:transform value
	AutomationsSort(source siteapi.SiteAutomationsSort) automations.Sort
	// goverter:enum:transform value
	SortDirection(source siteapi.SortDirection) pagination.Direction
	// goverter:enum:transform value
	InvitableRole(source siteapi.SiteInvitableRole) invitation.Role
	// goverter:enum:transform value
	MembershipRole(source siteapi.SiteMembershipRole) membership.Role
	// goverter:enum:transform value
	PersonalScope(source siteapi.PersonalScope) tokenscope.PersonalScope
	// goverter:enum:transform value
	RuleActionType(source siteapi.RuleActionType) ruleaction.Type
	// goverter:enum:transform value
	SignupCompanySize(source siteapi.SignupCompanySize) signupsurveyresponse.CompanySize
	// goverter:enum:transform value
	SignupGoal(source siteapi.SignupGoal) signupsurveyresponse.MainGoal
	// goverter:enum:transform value
	SignupRole(source siteapi.SignupRole) signupsurveyresponse.Role
	// goverter:enum:transform value
	SignupSource(source siteapi.SignupSource) signupsurveyresponse.HeardFrom
}

func stringPatch(o siteapi.OptString) patch.Field[string] { return convert.Patch(o) }
func namePatch(o siteapi.OptName) patch.Field[string]     { return convert.StringPatch(o) }
func nameIn(n siteapi.Name) string                        { return string(n) }
func namePtrIn(o siteapi.OptName) *string                 { return convert.StringPtr(o) }

// customFieldTypePatch is a Custom field's new type as the enum's value: the module reads it as its own.
func customFieldTypePatch(o siteapi.OptCustomFieldType) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilStringPatch(o siteapi.OptNilString) patch.Field[string]      { return convert.Patch(o) }
func nilEmailPatch(o siteapi.OptNilEmailAddress) patch.Field[string] { return convert.StringPatch(o) }
func nilTimeZonePatch(o siteapi.OptNilTimeZoneName) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilCountryPatch(o siteapi.OptNilCountryCode) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilLocalePatch(o siteapi.OptNilLocaleTag) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilImageURLPatch(o siteapi.OptNilImageUrl) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilDatePatch(o siteapi.OptNilDate) patch.Field[time.Time]        { return convert.Patch(o) }
func nilEntityIDPatch(o siteapi.OptNilEntityId) patch.Field[int64]    { return convert.Int64Patch(o) }
func boolPatch(o siteapi.OptBool) patch.Field[bool]                   { return convert.Patch(o) }
func maxPerSecondPatch(o siteapi.OptNilMaxPerSecond) patch.Field[int] { return convert.IntPatch(o) }
func maxPerDayPatch(o siteapi.OptNilMaxPerDay) patch.Field[int]       { return convert.IntPatch(o) }
func listPatch(list []string) patch.Field[[]string]                   { return convert.ListPatch(list) }
func stringIn(o siteapi.OptString) string                             { return o.Or("") }
func emailIn(o siteapi.OptEmailAddress) string                        { return string(o.Or("")) }
func signingSecretIn(o siteapi.OptSigningSecret) string               { return string(o.Or("")) }
func stringPtrIn(o siteapi.OptString) *string                         { return convert.Ptr(o) }
func emailPtrIn(o siteapi.OptEmailAddress) *string                    { return convert.StringPtr(o) }
func entityIDPtrIn(o siteapi.OptEntityId) *int64                      { return convert.Int64Ptr(o) }
func nilStringPtrIn(o siteapi.OptNilString) *string                   { return convert.Ptr(o) }
func nilEmailPtrIn(o siteapi.OptNilEmailAddress) *string              { return convert.StringPtr(o) }
func nilTimeZonePtrIn(o siteapi.OptNilTimeZoneName) *string           { return convert.StringPtr(o) }
func nilCountryPtrIn(o siteapi.OptNilCountryCode) *string             { return convert.StringPtr(o) }
func nilLocalePtrIn(o siteapi.OptNilLocaleTag) *string                { return convert.StringPtr(o) }
func nilImageURLPtrIn(o siteapi.OptNilImageUrl) *string               { return convert.StringPtr(o) }
func nilDatePtrIn(o siteapi.OptNilDate) *time.Time                    { return convert.Ptr(o) }
func nilEntityIDPtrIn(o siteapi.OptNilEntityId) *int64                { return convert.Int64Ptr(o) }

// requiredNilEntityIDPtrIn is a field the body must carry whose null is a choice of its own (a
// Broadcast's audience, null for all contacts): nil for null, else the id.
func requiredNilEntityIDPtrIn(o siteapi.NilEntityId) *int64              { return convert.Int64Ptr(o) }
func boolIn(o siteapi.OptBool) bool                                      { return o.Or(false) }
func boolPtrIn(o siteapi.OptBool) *bool                                  { return convert.Ptr(o) }
func entityIDIn(o siteapi.OptEntityId) int64                             { return int64(o.Or(0)) }
func maxPerSecondPtrIn(o siteapi.OptNilMaxPerSecond) *int                { return intPtr(o) }
func maxPerDayPtrIn(o siteapi.OptNilMaxPerDay) *int                      { return intPtr(o) }
func currencyCodePtrIn(o siteapi.OptCurrencyCode) *string                { return convert.StringPtr(o) }
func attributionWindowDaysPtrIn(o siteapi.OptAttributionWindowDays) *int { return intPtr(o) }
func int32ToInt(v int32) int                                             { return int(v) }

// listPtrIn is a list an update replaces: an omitted list (nil) keeps the stored one.
func listPtrIn(list []string) *[]string {
	if list == nil {
		return nil
	}
	return &list
}

// buttonsPtrIn is the link buttons a Telegram Broadcast's update replaces: an omitted list (nil)
// keeps the stored ones, an empty list removes them.
func buttonsPtrIn(c Inputs, list []siteapi.TelegramButtonInput) *[]telegramrender.Button {
	if list == nil {
		return nil
	}
	out := make([]telegramrender.Button, len(list))
	for i, b := range list {
		out[i] = c.TelegramButton(b)
	}
	return &out
}

func buttonsPatch(c Inputs, list []siteapi.TelegramButtonInput) patch.Field[[]telegramrender.Button] {
	return patch.FromPtr(buttonsPtrIn(c, list))
}

// timeIn is a required time as the module takes it.
func timeIn(v siteapi.Timestamp) time.Time { return time.Time(v) }

func timestampPtrIn(o siteapi.OptNilTimestamp) *time.Time {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	t := time.Time(v)
	return &t
}

// signupSourcePtrIn is where the User heard about the product, nil when the request does not say.
func signupSourcePtrIn(c Inputs, o siteapi.OptSignupSource) *signupsurveyresponse.HeardFrom {
	return convert.PtrVia(o, c.SignupSource)
}

// automationPathPtrIn is the root path a write replaces the steps with: omitted (nil) keeps the
// stored ones.
func automationPathPtrIn(c Inputs, o siteapi.OptAutomationPathInput) (*automationstep.Path, error) {
	in, ok := o.Get()
	if !ok {
		return nil, nil
	}
	out, err := c.AutomationPath(in)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// stepIDIn is the id a Step or a path is written with, "" when the request leaves it to the
// server.
func stepIDIn(o siteapi.OptStepId) automationstep.ID { return automationstep.ID(o.Or("")) }

// automationStepsIn maps a path's Steps onto the module's, each by automationStepIn.
func automationStepsIn(c Inputs, in []siteapi.AutomationStepInput) ([]automationstep.Step, error) {
	out := make([]automationstep.Step, len(in))
	for i := range in {
		st, err := automationStepIn(c, &in[i].OneOf)
		if err != nil {
			return nil, err
		}
		out[i] = st
	}
	return out, nil
}

// pathStepsIn maps the Steps of a fork's path onto the module's: the contract's union of a fork's
// path has the arms of the root's, so each is read as one of those (automationStepIn).
func pathStepsIn(c Inputs, in []siteapi.PathStepInput) ([]automationstep.Step, error) {
	out := make([]automationstep.Step, len(in))
	for i := range in {
		sum := c.PathStepInputSum(in[i].OneOf)
		st, err := automationStepIn(c, &sum)
		if err != nil {
			return nil, err
		}
		out[i] = st
	}
	return out, nil
}

// splitPathStepsIn maps the Steps of a Split's path onto the module's: the contract's union of a
// Split's path has the arms of the root's, so each is read as one of those (automationStepIn).
func splitPathStepsIn(c Inputs, in []siteapi.SplitPathStepInput) ([]automationstep.Step, error) {
	out := make([]automationstep.Step, len(in))
	for i := range in {
		sum := c.SplitPathStepInputSum(in[i].OneOf)
		st, err := automationStepIn(c, &sum)
		if err != nil {
			return nil, err
		}
		out[i] = st
	}
	return out, nil
}

// automationStepIn maps a Step of the union onto its kind's payload, with the id its arm carries;
// a oneOf is beyond goverter, so only the switch is written here.
func automationStepIn(c Inputs, s *siteapi.AutomationStepInputSum) (automationstep.Step, error) {
	switch s.Type {
	case siteapi.EmailStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.EmailStepInput.ID), Payload: c.EmailStepIn(s.EmailStepInput)}, nil
	case siteapi.TelegramStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.TelegramStepInput.ID), Payload: c.TelegramStepIn(s.TelegramStepInput)}, nil
	case siteapi.SmsStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.SmsStepInput.ID), Payload: c.SmsStepIn(s.SmsStepInput)}, nil
	case siteapi.WaitStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.WaitStepInput.ID), Payload: c.WaitStepIn(s.WaitStepInput)}, nil
	case siteapi.WaitUntilStepInputAutomationStepInputSum:
		w, err := c.WaitUntilStepIn(s.WaitUntilStepInput)
		return automationstep.Step{ID: stepIDIn(s.WaitUntilStepInput.ID), Payload: w}, err
	case siteapi.ApplyTagStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.ApplyTagStepInput.ID), Payload: c.ApplyTagStepIn(s.ApplyTagStepInput)}, nil
	case siteapi.RemoveTagStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.RemoveTagStepInput.ID), Payload: c.RemoveTagStepIn(s.RemoveTagStepInput)}, nil
	case siteapi.HttpStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.HttpStepInput.ID), Payload: c.HttpStepIn(s.HttpStepInput)}, nil
	case siteapi.EndStepInputAutomationStepInputSum:
		return automationstep.Step{ID: stepIDIn(s.EndStepInput.ID), Payload: automationstep.End{}}, nil
	case siteapi.BranchStepInputAutomationStepInputSum:
		return branchStepIn(c, &s.BranchStepInput)
	case siteapi.SplitStepInputAutomationStepInputSum:
		return splitStepIn(c, &s.SplitStepInput)
	default:
		return automationstep.Step{}, nil // the save refuses a Step of no kind
	}
}

// waitUntilMomentIn maps a Wait until's moment onto its form's: a oneOf is beyond goverter, so only
// the switch is written here.
func waitUntilMomentIn(c Inputs, in siteapi.WaitUntilMomentInput) (automationstep.Until, error) {
	switch in.OneOf.Type {
	case siteapi.WaitUntilTimeOfDayInputWaitUntilMomentInputSum:
		return c.WaitUntilTimeOfDayIn(in.OneOf.WaitUntilTimeOfDayInput), nil
	case siteapi.WaitUntilDateInputWaitUntilMomentInputSum:
		return automationstep.OnDate{Date: c.WaitUntilDateIn(in.OneOf.WaitUntilDateInput)}, nil
	default:
		return nil, fmt.Errorf("the moment of form %q has no form of a wait until", in.OneOf.Type)
	}
}

// branchStepIn maps a Branch: each listed path with its Condition (the contract's typed tree is
// the stored shape, ADR 0169) and its Steps, its otherwise and its unknown path.
func branchStepIn(c Inputs, in *siteapi.BranchStepInput) (automationstep.Step, error) {
	b := automationstep.Branch{Paths: make([]automationstep.BranchPath, len(in.Paths))}
	for i := range in.Paths {
		p := &in.Paths[i]
		cond, err := branchConditionIn(p.Condition)
		if err != nil {
			return automationstep.Step{}, err
		}
		steps, err := pathStepsIn(c, p.Steps)
		if err != nil {
			return automationstep.Step{}, err
		}
		b.Paths[i] = automationstep.BranchPath{ID: stepIDIn(p.ID), Condition: cond, Steps: steps}
	}
	otherwise, err := c.AutomationPath(in.Otherwise)
	if err != nil {
		return automationstep.Step{}, err
	}
	b.Otherwise = otherwise
	if u, ok := in.Unknown.Get(); ok {
		unknown, err := c.AutomationPath(u)
		if err != nil {
			return automationstep.Step{}, err
		}
		b.Unknown = &unknown
	}
	return automationstep.Step{ID: stepIDIn(in.ID), Payload: b}, nil
}

// splitStepIn maps a Split: each path with its weight and its Steps. The save checks the weights.
func splitStepIn(c Inputs, in *siteapi.SplitStepInput) (automationstep.Step, error) {
	sp := automationstep.Split{Paths: make([]automationstep.SplitPath, len(in.Paths))}
	for i := range in.Paths {
		p := &in.Paths[i]
		steps, err := splitPathStepsIn(c, p.Steps)
		if err != nil {
			return automationstep.Step{}, err
		}
		sp.Paths[i] = automationstep.SplitPath{ID: stepIDIn(p.ID), Weight: int(p.Weight), Steps: steps}
	}
	return automationstep.Step{ID: stepIDIn(in.ID), Payload: sp}, nil
}

// ruleActionsPtrIn is the Actions an update replaces: omitted (nil) keeps the stored ones.
func ruleActionsPtrIn(c Inputs, actions []siteapi.RuleActionInput) *[]ruleaction.Action {
	if actions == nil {
		return nil
	}
	out := c.RuleActions(actions)
	return &out
}

// segmentConditionIn is a Segment's Condition the request may leave out; nil when it is absent.
// branchConditionIn is a Branch path's Condition as the place's typed tree.
func branchConditionIn(in siteapi.BranchConditionInput) (conditiontree.Branch, error) {
	return condition.FromDTO[conditiontree.Branch](&in)
}

func segmentConditionIn(o siteapi.OptSegmentConditionInput) (*conditiontree.Segment, error) {
	return condition.FromOptionalDTO[conditiontree.Segment](o)
}

// automationTriggerIn is the Trigger a write gives, by its kind: an Event matcher, its filter the
// stored Condition of the place (ADR 0169), or a date (ADR 0126). A oneOf is beyond goverter, so the
// switch is written here; a Trigger of no kind is no matcher, which the save refuses.
func automationTriggerIn(c Inputs, t siteapi.AutomationTriggerInput) (automations.Trigger, error) {
	switch t.OneOf.Type {
	case siteapi.EventTriggerInputAutomationTriggerInputSum:
		ev := t.OneOf.EventTriggerInput
		filter, err := condition.FromOptionalDTO[conditiontree.EventFilter](ev.Filter)
		if err != nil {
			return nil, err
		}
		return automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: ev.Action, Filter: filter}}, nil
	case siteapi.DateTriggerInputAutomationTriggerInputSum:
		return automations.DateTrigger{Date: c.DateTriggerIn(t.OneOf.DateTriggerInput)}, nil
	}
	// A Trigger of no kind is an Event Trigger with no action, which the save refuses.
	return automations.EventTrigger{}, nil
}

// dateFieldIn is the date field a date Trigger names, by its kind: the birthday, or a Custom field by
// its key. A field of no kind names none, which the save refuses.
func dateFieldIn(f siteapi.DateFieldInput) datetrigger.Field {
	switch f.OneOf.Type {
	case siteapi.BirthdayDateFieldInputDateFieldInputSum:
		return datetrigger.Field{Kind: datetrigger.Birthday}
	case siteapi.CustomFieldDateFieldInputDateFieldInputSum:
		return datetrigger.Field{Kind: datetrigger.CustomField, Key: f.OneOf.CustomFieldDateFieldInput.Key}
	}
	return datetrigger.Field{}
}

// dateOffsetDaysIn is a date's offset in days, which the save holds to its bound.
func dateOffsetDaysIn(days siteapi.DateOffsetDays) int { return int(days) }

// automationEntryIn is the Entry policy and the Enrollment key a write gives with its Trigger, an
// Event Trigger's: an omitted field is left to the module's default, which for the key is none.
func automationEntryIn(c Inputs, t siteapi.AutomationTriggerInput) automations.Entry {
	if t.OneOf.Type != siteapi.EventTriggerInputAutomationTriggerInputSum {
		return automations.Entry{}
	}
	ev := t.OneOf.EventTriggerInput
	var e automations.Entry
	if policy, ok := ev.EntryPolicy.Get(); ok {
		e.Policy = c.EntryPolicy(policy)
	}
	if cooldown, ok := ev.Cooldown.Get(); ok {
		e.CooldownSeconds = new(int(cooldown))
	}
	if key, ok := ev.EnrollmentKey.Get(); ok {
		e.Key = &key
	}
	return e
}

// automationEntryPtrIn is the Entry policy an update replaces with its Trigger: omitted (nil) with
// the Trigger keeps the stored one.
func automationEntryPtrIn(c Inputs, o siteapi.OptAutomationTriggerInput) *automations.Entry {
	t, ok := o.Get()
	if !ok {
		return nil
	}
	return new(automationEntryIn(c, t))
}

// eventMatcherIn is an exit or goal event a write gives: an Event matcher, its filter the stored
// Condition of the Event filter's place (ADR 0133, ADR 0169).
func eventMatcherIn(m siteapi.EventMatcherInput) (eventmatcher.Matcher, error) {
	filter, err := condition.FromOptionalDTO[conditiontree.EventFilter](m.Filter)
	if err != nil {
		return eventmatcher.Matcher{}, err
	}
	return eventmatcher.Matcher{Action: m.Action, Filter: filter}, nil
}

// automationExitsIn is the exit events a write gives; nil when it gives none.
func automationExitsIn(exits []siteapi.EventMatcherInput) ([]eventmatcher.Matcher, error) {
	if exits == nil {
		return nil, nil
	}
	out := make([]eventmatcher.Matcher, len(exits))
	for i, e := range exits {
		m, err := eventMatcherIn(e)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// automationExitsPtrIn is the exit events an update replaces: an omitted list (nil) keeps the
// stored ones, an empty list removes them.
func automationExitsPtrIn(exits []siteapi.EventMatcherInput) (*[]eventmatcher.Matcher, error) {
	out, err := automationExitsIn(exits)
	if err != nil || out == nil {
		return nil, err
	}
	return &out, nil
}

// automationGoalIn is the goal event a write gives; nil for null or omitted, which is none.
func automationGoalIn(o siteapi.OptNilEventMatcherInput) (*eventmatcher.Matcher, error) {
	g, ok := o.Get()
	if !ok {
		return nil, nil
	}
	m, err := eventMatcherIn(g)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// automationGoalPatch is the goal event an update writes: omitted keeps it, null removes it.
func automationGoalPatch(o siteapi.OptNilEventMatcherInput) (patch.Field[eventmatcher.Matcher], error) {
	g, err := automationGoalIn(o)
	if err != nil {
		return patch.Keep[eventmatcher.Matcher](), err
	}
	if g == nil {
		return patch.Of(eventmatcher.Matcher{}, o.IsSet(), true), nil
	}
	return patch.Set(*g), nil
}

// automationTriggerPtrIn is the Trigger an update replaces: omitted (nil) keeps the stored one.
func automationTriggerPtrIn(c Inputs, o siteapi.OptAutomationTriggerInput) (automations.Trigger, error) {
	t, ok := o.Get()
	if !ok {
		return nil, nil
	}
	return automationTriggerIn(c, t)
}

// ruleConditionIn is a Rule's Condition the request may leave out; nil when it is absent.
func ruleConditionIn(o siteapi.OptRuleConditionInput) (*conditiontree.Rule, error) {
	return condition.FromOptionalDTO[conditiontree.Rule](o)
}

// documentIn is the editor document of a write as the JSON the modules render; nil when the
// request names none.
func documentIn(o siteapi.OptEditorDocument) json.RawMessage { return convert.DocJSON(o.Value, o.Set) }

// documentPatch is the editor document of an update: an absent key keeps the stored document.
func documentPatch(o siteapi.OptEditorDocument) patch.Field[json.RawMessage] {
	return convert.DocPatch(o)
}

// objectIn and nilObjectIn are an opaque object of a write as plain values; nil when absent or null.
func objectIn(o siteapi.OptJsonObject) map[string]any       { return convert.RawMap(o.Or(nil)) }
func nilObjectIn(o siteapi.OptNilJsonObject) map[string]any { return convert.RawMap(o.Or(nil)) }

// objectPatch and nilObjectPatch are an opaque object of an update: an absent key keeps, an
// explicit null clears.
func objectPatch(o siteapi.OptJsonObject) patch.Field[map[string]any] { return convert.MapPatch(o) }
func nilObjectPatch(o siteapi.OptNilJsonObject) patch.Field[map[string]any] {
	return convert.MapPatch(o)
}

func intPtr[T ~int32](o interface{ Get() (T, bool) }) *int {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	n := int(v)
	return &n
}

// errUnknownProvider: a config whose kind the contract does not know, which the decoder never
// lets through.
var errUnknownProvider = errors.New("unsupported provider")

// integrationConfigIn maps the typed config union onto the module's provider-neutral input; a
// oneOf is beyond goverter, so only the switch is written here.
func integrationConfigIn(in siteapi.IntegrationConfigInput) (integrations.ConfigInput, error) {
	switch in.OneOf.Type {
	case siteapi.SmtpConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SmtpConfigInput
		return integrations.ConfigInput{SMTP: &integrations.SMTPConfig{
			Host: c.Host, Port: int(c.Port), Username: c.Username.Or(""), Password: c.Password.Or(""),
			From: string(c.From), FromName: c.FromName.Or(""),
		}}, nil
	case siteapi.SesConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SesConfigInput
		return integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: c.Region, AccessKeyID: c.AccessKeyId, SecretAccessKey: c.SecretAccessKey,
			From: string(c.From), FromName: c.FromName.Or(""), Endpoint: c.Endpoint.Or(""),
			ConfigurationSet: string(c.ConfigurationSet.Or("")),
		}}, nil
	case siteapi.InboxConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Inbox: &integrations.InboxConfig{}}, nil
	case siteapi.TelegramConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Telegram: &integrations.TelegramConfig{Token: in.OneOf.TelegramConfigInput.Token}}, nil
	case siteapi.TwilioConfigInputIntegrationConfigInputSum:
		c := in.OneOf.TwilioConfigInput
		return integrations.ConfigInput{Twilio: &integrations.TwilioConfig{
			AccountSID: c.AccountSid, APIKeySID: c.ApiKeySid.Or(""), Secret: c.Secret, From: c.From,
		}}, nil
	}
	return integrations.ConfigInput{}, errUnknownProvider
}

func integrationConfigPatch(o siteapi.OptIntegrationConfigInput) (patch.Field[integrations.ConfigInput], error) {
	v, ok := o.Get()
	if !ok {
		return patch.Keep[integrations.ConfigInput](), nil
	}
	cfg, err := integrationConfigIn(v)
	if err != nil {
		return patch.Field[integrations.ConfigInput]{}, err
	}
	return patch.Set(cfg), nil
}

// errUnknownChannel: content whose channel the contract does not know, which the decoder never
// lets through.
var errUnknownChannel = errors.New("unsupported channel")

// broadcastContentIn maps a Broadcast's content union onto the module's input of its Channel; a
// oneOf is beyond goverter, so only the switch is written here.
func broadcastContentIn(c Inputs, in siteapi.BroadcastContentInput) (broadcasts.ContentInput, error) {
	switch in.OneOf.Type {
	case siteapi.EmailBroadcastContentInputBroadcastContentInputSum:
		return c.EmailBroadcastContentIn(in.OneOf.EmailBroadcastContentInput), nil
	case siteapi.TelegramBroadcastContentInputBroadcastContentInputSum:
		return c.TelegramBroadcastContentIn(in.OneOf.TelegramBroadcastContentInput), nil
	case siteapi.SmsBroadcastContentInputBroadcastContentInputSum:
		return c.SmsBroadcastContentIn(in.OneOf.SmsBroadcastContentInput), nil
	default:
		return nil, errUnknownChannel
	}
}

// broadcastContentPatch is the content of an update: absent keeps the content.
func broadcastContentPatch(c Inputs, o siteapi.OptBroadcastContentInput) (broadcasts.ContentPatch, error) {
	in, ok := o.Get()
	if !ok {
		return nil, nil
	}
	switch in.OneOf.Type {
	case siteapi.EmailBroadcastContentInputBroadcastContentInputSum:
		return c.EmailBroadcastContentPatch(in.OneOf.EmailBroadcastContentInput), nil
	case siteapi.TelegramBroadcastContentInputBroadcastContentInputSum:
		return c.TelegramBroadcastContentPatch(in.OneOf.TelegramBroadcastContentInput), nil
	case siteapi.SmsBroadcastContentInputBroadcastContentInputSum:
		return c.SmsBroadcastContentPatch(in.OneOf.SmsBroadcastContentInput), nil
	default:
		return nil, errUnknownChannel
	}
}

// broadcastChannelPtrIn is the Channel a list is narrowed to: nil for every Channel.
func broadcastChannelPtrIn(c Inputs, o siteapi.OptBroadcastChannel) *channels.Channel {
	return convert.PtrVia(o, c.BroadcastChannel)
}

// automationStateIn is the State the Automation list is narrowed to: empty for every State.
func automationStateIn(c Inputs, o siteapi.OptSiteAutomationState) automations.State {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.AutomationState(v)
}

// enrollmentStatusIn is the status the Enrollment list is narrowed to: empty for every status.
func enrollmentStatusIn(o siteapi.OptEnrollmentStatus) string { return string(o.Or("")) }

// automationsSortIn is what the Automation list is ordered by: empty for the default.
func automationsSortIn(c Inputs, o siteapi.OptSiteAutomationsSort) automations.Sort {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.AutomationsSort(v)
}

// sortDirectionIn is the direction of a list's order: empty for the list's default.
func sortDirectionIn(c Inputs, o siteapi.OptSortDirection) pagination.Direction {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.SortDirection(v)
}

func createSegmentCondition(g siteapi.SegmentConditionInput) (*conditiontree.Segment, error) {
	def, err := condition.FromDTO[conditiontree.Segment](&g)
	if err != nil {
		return nil, err
	}
	return &def, nil
}

func segmentConditionPatch(o siteapi.OptSegmentConditionInput) (patch.Field[conditiontree.Segment], error) {
	def, err := condition.FromOptionalDTO[conditiontree.Segment](o)
	if err != nil {
		return patch.Field[conditiontree.Segment]{}, err
	}
	return patch.FromPtr(def), nil
}

// revisionIn is the revision an edit names (ADR 0119), as the module takes it.
func revisionIn(v siteapi.Revision) int64 {
	return int64(v)
}

// revisionPtrIn is the revision an optional-revision input names (ADR 0119).
func revisionPtrIn(v siteapi.Revision) *int64 {
	r := int64(v)
	return &r
}
