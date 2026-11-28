// Package resources holds goverter-generated mappers from ent entities to site
// API resources (the ogen *Resource DTOs), plus the custom conversions goverter
// cannot infer on its own. The generated implementation (ConverterImpl) lives in
// converter_gen.go; consumers instantiate it (see internal/api/site).
package resources

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/contactimport"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/engagementtrackingconsent"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactimports"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactsexport"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/insights/abusewarning"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/reputation"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"

	"github.com/go-faster/jx"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/identity/consentrecords"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/identity/signinprovider"
	"github.com/getsphericon/sphericon/internal/domain/identity/signupitems"
	"github.com/getsphericon/sphericon/internal/domain/insights/setupchecklist"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/csvimport"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/kernel/tokenscope"
)

// Converter maps ent entities to site API resources. goverter generates the
// implementation; named string/time casts are inferred, the extends below cover
// the conversions it can't (ids, optionals, the event property bag).
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend revision
// goverter:extend nilFloat64
// goverter:extend optNilString
// goverter:extend optNilStrings
// goverter:extend optNilStepID
// goverter:extend optNilOptOutVia
// goverter:extend optNilEngagementTrackingProvenance
// goverter:extend optNilHoldReason
// goverter:extend omitEmpty
// goverter:extend omitEmptyConfigurationSet
// goverter:extend optNilEmailAddress
// goverter:extend optNilEntityID
// goverter:extend optEntityID
// goverter:extend optStepID
// goverter:extend optNilTimeZone
// goverter:extend optNilCountry
// goverter:extend optNilLocale
// goverter:extend optNilImageURL
// goverter:extend optNilDate
// goverter:extend nilCurrencyCode
// goverter:extend nilTimeZone
// goverter:extend nilRegionalFormat
// goverter:extend attributionWindowDays
// goverter:extend nilCountryCode
// goverter:extend nilFrequencyCapMaxMessages
// goverter:extend nilFrequencyCapPeriodMinutes
// goverter:extend minuteOfDay
// goverter:extend workspaceTimezone
// goverter:extend optNilTimestamp
// goverter:extend nilTimestamp
// goverter:extend jsonObject
// goverter:extend editorDocument
// goverter:extend automationPath
// goverter:extend automationStepViews
// goverter:extend emailStepType
// goverter:extend telegramStepType
// goverter:extend smsStepType
// goverter:extend waitStepType
// goverter:extend waitUntilStepType
// goverter:extend waitUntilMoment
// goverter:extend dateField
// goverter:extend dateOffsetDays
// goverter:extend dateTriggerKind
// goverter:extend waitUntilDateForm
// goverter:extend waitUntilTimeOfDayForm
// goverter:extend applyTagStepType
// goverter:extend removeTagStepType
// goverter:extend httpStepType
// goverter:extend endStepType
// goverter:extend branchStepType
// goverter:extend splitStepType
// goverter:extend waitForEventStepType
// goverter:extend stepName
// goverter:extend waitForEventAwaits
// goverter:extend splitPathStepViews
// goverter:extend pathStepViews
// goverter:extend optAutomationPath
// goverter:extend branchCondition
// goverter:extend segmentCondition
// goverter:extend ruleCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend emailVerified
// goverter:extend transactionalChannel
// goverter:extend transactionalStatus
// goverter:extend transactionalError
// goverter:extend nilInt32
// goverter:extend nilFloat32
// goverter:extend nilString
// goverter:extend count
// goverter:extend maxPerDay
// goverter:extend providerLimitSource
// goverter:extend broadcastLimit
// goverter:extend optNilSendCap
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optEmail
// goverter:extend optEditorDocument
// goverter:extend optMessageRevenue
// goverter:extend optAttentionSubject
// goverter:extend optAbuseWarning
// goverter:extend optTrackingReason
// goverter:extend optSkippedReason
// goverter:extend optDeferralCause
// goverter:extend timestampOf
// goverter:extend tagNames
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend optConsentDecision
// goverter:extend durationSeconds
// goverter:extend broadcastContent
// goverter:extend emailContentChannel
// goverter:extend telegramContentChannel
// goverter:extend smsContentChannel
// goverter:extend optNilSmsConfirmation
// goverter:extend optNilSmsOptOut
type Converter interface {
	// The contact is read with its tags loaded (`WithTags`).
	// goverter:map Edges.Tags Tags
	ContactToResource(source *ent.Contact) siteapi.SiteContactResource
	// goverter:useEmptyOnNil:slice
	ContactsToResources(source []*ent.Contact) []siteapi.SiteContactResource
	SegmentToResource(source *ent.Segment) (siteapi.SiteSegmentResource, error)
	// goverter:useEmptyOnNil:slice
	SegmentsToResources(source []*ent.Segment) ([]siteapi.SiteSegmentResource, error)
	TokenToResource(source *ent.ApiToken) siteapi.SiteApiTokenResource
	// goverter:useEmptyOnNil:slice
	TokensToResources(source []*ent.ApiToken) []siteapi.SiteApiTokenResource
	// A Connection is an API token read with its Membership and that Membership's User loaded
	// (apitokens.Module.Connections); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	ConnectionToResource(source *ent.ApiToken) siteapi.SiteConnectionResource
	// goverter:useEmptyOnNil:slice
	ConnectionsToResources(source []*ent.ApiToken) []siteapi.SiteConnectionResource
	// A Personal token is an API token read with its Membership and that Membership's User loaded
	// (apitokens.Module.PersonalTokens, MintPersonal); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	PersonalTokenToResource(source *ent.ApiToken) siteapi.SitePersonalTokenResource
	// goverter:useEmptyOnNil:slice
	PersonalTokensToResources(source []*ent.ApiToken) []siteapi.SitePersonalTokenResource

	// Verification is derived: a non-nil email_verified_at means verified.
	// The Sign-up survey is due only while the Instance still asks it, which the mapper cannot
	// see: the handler sets it from the marker and the Instance switch.
	// goverter:map EmailVerifiedAt EmailVerified | emailVerified
	// goverter:ignore SignupSurveyDue
	// The pending acceptances are read from the Consent records and the Sign-up items in effect,
	// which the mapper cannot see: the handler attaches them (SignupItemsToSite).
	// goverter:ignore AcceptancesPending
	UserToResource(source *ent.User) *siteapi.SiteUserResource
	// The provider is shown by its display name, which the Instance's registry holds and the
	// mapper cannot see: the handler sets it.
	// goverter:ignore Provider
	PendingSignupToResource(source *ent.PendingSignup) *siteapi.SitePendingSignup
	// The grace end, the role and its Permissions belong to the authenticated User's Membership,
	// which the mapper cannot see: the handler attaches them.
	// goverter:ignore SecondFactorGraceEndsAt
	// The requirement's grace end is the Two-factor module's rule, read into the summary: the
	// handler attaches it.
	// goverter:ignore SecondFactorRequirementGraceEndsAt
	// goverter:ignore Role
	// goverter:ignore Permissions
	// goverter:ignore AccountLinkAvailable
	// The active holds are read through the Workspace's scope, which the mapper cannot
	// see: the handler attaches them.
	// goverter:ignore Holds
	// The support contact is instance configuration, attached by the handler.
	// goverter:ignore SupportEmail
	// The MCP origin is instance configuration, attached by the handler.
	// goverter:ignore McpUrl
	// goverter:ignore TrackerUrl
	// goverter:ignore TrackingTarget
	// The Send cap shown is the one the send path enforces (outboundmessages.Module.SendCapStatus), which
	// the mapper cannot see: the handler attaches it through SendCapStatusToResource.
	// goverter:ignore SendCap
	// The length of a short tracked link depends on the Workspace's serving Tracking domain
	// (outboundmessages.Module.ShortLinkLength): the handler attaches it.
	// goverter:ignore ShortLinkLength
	// The SMS provider's length limit is the send path's (outboundmessages.Module.SMSLengthLimit): the
	// handler attaches it through SmsLengthLimitToResource.
	// goverter:ignore SmsMaxLength
	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	WorkspaceToResource(source *ent.Workspace) siteapi.SiteWorkspaceResource
	// The revenue settings are two columns of the Workspace (ADR 0064).
	RevenueSettingsToResource(source *ent.Workspace) siteapi.SiteRevenueSettingsResource
	// The Workspace's sending rules (ADR 0134, ADR 0137) with the revision they were read at.
	// goverter:useEmptyOnNil:slice
	SendingRulesToResource(source workspaces.SendingRules) siteapi.SiteSendingRulesResource
	FrequencyCapToResource(source workspaces.FrequencyCap) siteapi.FrequencyCap
	// goverter:useEmptyOnNil:slice
	SendingWindowToResource(source workspaces.SendingWindow) siteapi.SendingWindow
	SendingWindowHoursToResource(source workspaces.OpenHours) siteapi.SendingWindowHours
	// goverter:enum:transform value
	FrequencyCapChannelToResource(source channels.Channel) siteapi.FrequencyCapChannel
	// Only SMS has a Sending window (workspaces.HasSendingWindow), as the rows' CHECK holds.
	// goverter:enum:transform value
	// goverter:enum:map Email @panic
	// goverter:enum:map Telegram @panic
	SendingWindowChannelToResource(source channels.Channel) siteapi.SendingWindowChannel
	// The confirmation email's settings (ADR 0013): what the Workspace stores beside the defaults,
	// null where it keeps the default.
	// goverter:map FromEmail | nilEmailAddress
	// goverter:map FromName | nilString
	// goverter:map Subject | nilString
	// goverter:map Intro | nilString
	ConfirmationEmailToResource(source workspaces.ConfirmationEmailSettings) siteapi.SiteConfirmationEmailResource
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:map Currency ReportingCurrency
	// goverter:useEmptyOnNil:slice
	RevenueToResource(source *orders.WorkspaceRevenue) siteapi.SiteAnalyticsRevenue
	// goverter:useEmptyOnNil:slice
	RevenuePeriodToResource(source orders.RevenuePeriod) siteapi.SiteAnalyticsRevenuePeriod

	// The analytics reports map whole from the module's report, so a field the response gains
	// with no source fails generation. A figure the module does not hold (the Event retention) is
	// attached by the handler.
	// goverter:useEmptyOnNil:slice
	// goverter:map Series Timeseries
	AnalyticsOverviewToResource(source *analyticsreport.Overview) *siteapi.SiteAnalyticsOverview
	// goverter:map Window WindowHours | windowHours
	// goverter:useEmptyOnNil:slice
	AnalyticsDeliverabilityToResource(source analyticsreport.Deliverability) siteapi.SiteAnalyticsDeliverability
	// goverter:map SendingDomainID SendingDomainId
	// goverter:map Complaint ComplaintRate
	// goverter:map Bounce BounceRate
	AnalyticsDomainRatesToResource(source analyticsreport.DomainRates) siteapi.SiteSendingDomainRates
	AnalyticsRateToResource(source reputation.Rate) siteapi.RateTriple
	AnalyticsContactsToResource(source analyticsreport.Contacts) siteapi.SiteAnalyticsContacts
	AnalyticsPeriodToResource(source analyticsreport.Period) siteapi.SiteAnalyticsPeriod
	// goverter:map Sent SentCount
	// goverter:map Opened OpenedCount
	// goverter:map Clicked ClickedCount
	// goverter:map Untracked UntrackedCount
	AnalyticsEmailToResource(source analyticsreport.Email) siteapi.SiteAnalyticsEmail
	// The Events and Clicks by link reports are lists, which goverter cannot map onto their
	// envelopes: the handler builds those, of one list each.
	// goverter:useEmptyOnNil:slice
	AnalyticsEventActionsToResources(source []analyticsreport.EventAction) []siteapi.SiteAnalyticsEventAction
	// goverter:useEmptyOnNil:slice
	AnalyticsSubscribedPeopleToResource(source *analyticsreport.SubscribedPeople) *siteapi.SiteAnalyticsSubscribedPeople
	// goverter:useEmptyOnNil:slice
	AnalyticsLinksToResources(source []analyticsreport.LinkClicks) []siteapi.SiteAnalyticsLink
	// goverter:useEmptyOnNil:slice
	AnalyticsOpensByHourToResource(source *analyticsreport.OpensByHour) *siteapi.SiteAnalyticsOpensByHour
	// goverter:useEmptyOnNil:slice
	// goverter:map Series Timeseries
	AnalyticsNewPeopleToResource(source *analyticsreport.NewPeople) *siteapi.SiteNewPeopleReport
	AbuseWarningToResource(source abusewarning.Warning) siteapi.AbuseWarning

	// What needs attention maps whole from the module's items; an absent subject, start, hold
	// reason or rate is left out.
	// goverter:useEmptyOnNil:slice
	AttentionItemsToResources(source []needsattentionreport.Item) []siteapi.SiteAttentionItem
	AttentionSubjectToResource(source needsattentionreport.Subject) siteapi.SiteAttentionSubject

	// The email's subject is not on the Event: the handler reads it from the Outbound message.
	// goverter:ignore EmailSubject
	// goverter:map . Properties | eventProperties
	EventToResource(source *ent.Event) siteapi.SiteEventResource
	// goverter:useEmptyOnNil:slice
	EventsToResources(source []*ent.Event) []siteapi.SiteEventResource
	LinkClicksToResource(source []linkclick.Clicks) []siteapi.LinkClicks
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:autoMap Credited
	// goverter:map Currency ReportingCurrency
	MessageRevenueToResource(source orders.Revenue) siteapi.MessageRevenue
	AutomationReportToResource(source automations.Report) siteapi.AutomationReport
	// goverter:autoMap ResultCounts
	// goverter:map AverageDuration AverageDurationSeconds
	AutomationPeriodResultsToResource(source automations.PeriodResults) siteapi.AutomationPeriodResults
	// goverter:autoMap ResultCounts
	AutomationResultsPointToResource(source automations.ResultsPoint) siteapi.AutomationResultsPoint
	// The period is the one the request named: the handler attaches it.
	AutomationResultsToResource(source automations.Results) siteapi.AutomationResults

	// The delivery figures are derived from the Broadcast's recipients and their Outbound
	// messages, which the mapper cannot see: the handler attaches them (BroadcastStatsToResource).
	// goverter:ignore Stats
	// Progress is derived from the recipients, which the mapper cannot see: the
	// handler attaches it.
	// goverter:ignore Progress
	// Warnings are the lint of the content just saved (ADR 0040): the handler attaches them
	// to the response of a save.
	// goverter:ignore Warnings
	// Recipients are counted by Schedule (ADR 0149): the handler attaches them to its answer.
	// goverter:ignore Recipients
	// The unsent are counted by Cancel (ADR 0150): the handler attaches them to its answer.
	// goverter:ignore Unsent
	// goverter:map . Content | broadcastContent
	BroadcastToResource(source *ent.Broadcast) siteapi.SiteBroadcastResource
	// The email content of an email Broadcast, the arm of its content that broadcastContent wraps.
	EmailBroadcastContentToResource(source *ent.Broadcast) siteapi.EmailBroadcastContent
	// The Telegram content of a Telegram Broadcast, the arm of its content that broadcastContent wraps.
	TelegramBroadcastContentToResource(source *ent.Broadcast) siteapi.TelegramBroadcastContent
	// The SMS content of an SMS Broadcast, the arm of its content that broadcastContent wraps.
	SmsBroadcastContentToResource(source *ent.Broadcast) siteapi.SmsBroadcastContent

	// A Broadcast's delivery figures as its Channel reports them: a figure it does not report is
	// null. The rates are ratios in [0,1].
	// goverter:map Recipients RecipientsTotal
	// goverter:map Sent SentCount
	// goverter:map Skipped SkippedCount
	// goverter:map Pending PendingCount
	// goverter:map Opened OpenedCount
	// goverter:map Clicked ClickedCount
	// goverter:map Unsubscribed UnsubscribedCount
	// goverter:map Failed FailedCount
	// goverter:map Cancelled CancelledCount
	// goverter:map Delivered DeliveredCount
	// goverter:map Undelivered UndeliveredCount
	// goverter:map Bounced BouncedCount
	// goverter:map Complained ComplainedCount
	// goverter:map Captured CapturedCount
	// goverter:map Untracked UntrackedCount
	// goverter:map SingleZone SingleZoneCount
	BroadcastStatsToResource(source broadcasts.Figures) siteapi.SiteBroadcastStats

	// goverter:map Processed ProcessedCount
	// goverter:map Remaining RemainingCount
	// goverter:map EstimatedCompletion EstimatedCompletionAt
	BroadcastProgressToResource(source broadcasts.Progress) siteapi.SiteBroadcastProgress
	// A Broadcast's progress names only the Send cap or the Provider limit, never the provider's
	// own throttling nor a per-recipient cause, which only a Deferral reports.
	// goverter:enum:map SendCap SiteBroadcastLimitSendCap
	// goverter:enum:map ProviderLimit SiteBroadcastLimitProviderLimit
	// goverter:enum:map SlowDown @panic
	// goverter:enum:map SendingWindow @panic
	// goverter:enum:map FrequencyCap @panic
	BroadcastLimitToResource(source deferralcause.Cause) siteapi.SiteBroadcastLimit

	// goverter:ignore Warnings
	EmailTemplateToResource(source *ent.EmailTemplate) siteapi.SiteEmailTemplateResource
	// goverter:useEmptyOnNil:slice
	EmailTemplatesToResources(source []*ent.EmailTemplate) []siteapi.SiteEmailTemplateResource

	// A lint finding (ADR 0040) names its element and detail only when the check has one.
	// goverter:map Element Element | optNonEmpty
	// goverter:map Detail Detail | optNonEmpty
	LintFindingToResource(source emailrender.Finding) siteapi.LintFinding
	// goverter:useEmptyOnNil:slice
	LintFindingsToResource(source []emailrender.Finding) []siteapi.LintFinding

	// The live version and the draft are the edges the automations module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	// The revenue is summed from Order Movements, which the mapper cannot see: an Automation the
	// automations module read with its revenue maps through AutomationWithRevenueToResource.
	// goverter:ignore Revenue
	AutomationToResource(source *ent.Automation) (siteapi.SiteAutomationResource, error)
	// An Automation with the revenue the automations module read for it (ADR 0064).
	// goverter:autoMap Automation
	// goverter:map Automation.Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Automation.Edges.DraftVersion Draft | automationVersionOrNull
	AutomationWithRevenueToResource(source automations.WithRevenue) (siteapi.SiteAutomationResource, error)
	// goverter:useEmptyOnNil:slice
	AutomationsWithRevenueToResources(source []automations.WithRevenue) ([]siteapi.SiteAutomationResource, error)
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map Steps | automationPath
	// goverter:map . Trigger | automationTrigger
	// goverter:map . ExitEvents | automationExits
	// goverter:map Goal GoalEvent | automationGoal
	AutomationVersionToResource(source *ent.AutomationVersion) (siteapi.AutomationVersion, error)
	// goverter:map Steps | automationStepViews
	AutomationPathToResource(source automations.PathView) (siteapi.AutomationPath, error)
	// The arms of a Step, the module's masked view of each kind, which automationStepViews wraps.
	// goverter:map IntegrationID IntegrationId
	EmailStepToResource(source automations.EmailView) siteapi.EmailStep
	// goverter:map IntegrationID IntegrationId
	TelegramStepToResource(source automations.TelegramView) siteapi.TelegramStep
	// goverter:map IntegrationID IntegrationId
	SmsStepToResource(source automations.SMSView) siteapi.SmsStep
	WaitStepToResource(source automations.WaitView) siteapi.WaitStep
	WaitUntilStepToResource(source automations.WaitUntilView) (siteapi.WaitUntilStep, error)
	// goverter:map . Form | waitUntilTimeOfDayForm
	// goverter:useEmptyOnNil:slice
	WaitUntilTimeOfDayToResource(source automationstep.TimeOfDay) siteapi.WaitUntilTimeOfDay
	// goverter:enum:transform value
	WeekdayToResource(source localcalendar.Weekday) siteapi.Weekday
	ApplyTagStepToResource(source automations.TagView) siteapi.ApplyTagStep
	RemoveTagStepToResource(source automations.TagView) siteapi.RemoveTagStep
	HttpStepToResource(source automations.HTTPView) siteapi.HttpStep
	EndStepToResource(source automations.EndView) siteapi.EndStep
	BranchStepToResource(source automations.BranchView) (siteapi.BranchStep, error)
	BranchPathToResource(source automations.BranchPathView) (siteapi.BranchPath, error)
	// A Step of the root's union as a Step of a fork's path: the two unions have the same arms, so
	// an arm one of them lacks fails generation (pathStepViews).
	// goverter:skipCopySameType
	PathStepSum(source siteapi.AutomationStepSum) siteapi.PathStepSum
	// goverter:enum:transform value
	PathStepType(source siteapi.AutomationStepSumType) siteapi.PathStepSumType
	// A Step of the root's union as a Step of a Split's path, as PathStepSum (splitPathStepViews).
	// goverter:skipCopySameType
	SplitPathStepSum(source siteapi.AutomationStepSum) siteapi.SplitPathStepSum
	// goverter:enum:transform value
	SplitPathStepType(source siteapi.AutomationStepSumType) siteapi.SplitPathStepSumType
	SplitStepToResource(source automations.SplitView) (siteapi.SplitStep, error)
	WaitForEventStepToResource(source automations.WaitForEventView) (siteapi.WaitForEventStep, error)
	SplitPathToResource(source automations.SplitPathView) (siteapi.SplitPath, error)
	// goverter:ignore Value
	AutomationHttpHeaderToResource(source automations.HeaderName) siteapi.AutomationHttpHeader
	TelegramButtonToResource(source telegramrender.Button) siteapi.TelegramButton

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Provider limit from providerlimit.Module.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore ProviderLimit
	IntegrationToResource(source *ent.Integration) siteapi.SiteIntegrationResource
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) siteapi.SmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) siteapi.SesConfig
	// goverter:map . Kind | telegramConfigKind
	// goverter:map BotID BotId
	TelegramConfigToResource(source integrations.RedactedTelegram) siteapi.TelegramConfig
	// goverter:enum:transform value
	EntryPolicyToResource(source automationversion.EntryPolicy) siteapi.EntryPolicy
	// goverter:enum:transform value
	DateRecurrenceToResource(source localcalendar.Recurrence) siteapi.DateRecurrence
	// A date Trigger and a Wait until on a date show the same occurrence of a date (ADR 0126).
	// goverter:map . Kind | dateTriggerKind
	DateTriggerToResource(source datetrigger.Date) siteapi.DateTrigger
	// goverter:map . Form | waitUntilDateForm
	WaitUntilDateToResource(source datetrigger.Date) siteapi.WaitUntilDate
	// goverter:enum:transform value
	SmsFromKindToResource(source messaging.SMSFromKind) siteapi.SmsFromKind
	// goverter:map . Kind | twilioConfigKind
	// goverter:map AccountSID AccountSid
	// goverter:map . ApiKeySid | twilioAPIKeySID
	TwilioConfigToResource(source integrations.RedactedTwilio) siteapi.TwilioConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) siteapi.ProviderLimitStatus
	// goverter:map Sent24h SentLast24h
	SendCapToResource(source providerlimit.SendCapStatus) siteapi.SendCap
	// The Send cap that applies (outboundmessages.Module.SendCapStatus), null when none does.
	SendCapStatusToResource(source *providerlimit.SendCapStatus) siteapi.OptNilSendCap
	PasskeyToResource(source *ent.Passkey) siteapi.SitePasskey
	// goverter:useEmptyOnNil:slice
	PasskeysToResources(source []*ent.Passkey) []siteapi.SitePasskey
	// A Linked identity's display name is the provider registry's (ADR 0048), not a column.
	// goverter:map Provider DisplayName | providerDisplayName
	IdentityToResource(source *ent.UserIdentity) siteapi.SiteLinkedIdentity
	// goverter:useEmptyOnNil:slice
	IdentitiesToResources(source []*ent.UserIdentity) []siteapi.SiteLinkedIdentity
	SuppressionToResource(source *ent.Suppression) siteapi.SiteSuppressionResource
	// goverter:useEmptyOnNil:slice
	SuppressionsToResources(source []*ent.Suppression) []siteapi.SiteSuppressionResource
	// goverter:useEmptyOnNil:slice
	ConditionTestsToResource(source condition.Tests) siteapi.CustomFieldConditionTests
	// goverter:enum:transform value
	TextOperatorToResource(source conditiontree.TextOperator) siteapi.TextOperator
	// goverter:enum:transform value
	MessageEngagementToResource(source conditiontree.Engagement) siteapi.MessageEngagement
	// goverter:enum:transform value
	NumberOperatorToResource(source conditiontree.NumberOperator) siteapi.NumberOperator
	// goverter:enum:transform value
	MomentOperatorToResource(source conditiontree.MomentOperator) siteapi.MomentOperator
	// goverter:enum:transform value
	PresenceOperatorToResource(source conditiontree.PresenceOperator) siteapi.PresenceOperator
	// goverter:map Type ConditionTests | customFieldConditionTests
	CustomFieldToResource(source *ent.CustomField) siteapi.SiteCustomFieldResource

	// A CSV import (ADR 0138): its row, the mapping it reads its file by (the saved one or the
	// proposal) with its "only add new" choice, what its run wrote and its report's rows.
	// goverter:autoMap Import
	// goverter:map Import.RowCount Rows
	// goverter:map . Mapping | contactImportMapping
	// goverter:map Import Counts
	// goverter:map Import.StartedAt StartedAt
	// goverter:map Import.EndedAt EndedAt
	// goverter:map Import.CancelRequestedAt CancelRequestedAt
	// goverter:useEmptyOnNil:slice
	ContactImportToResource(source contactimports.View) siteapi.SiteContactImport
	ContactImportCounts(source *ent.ContactImport) siteapi.ContactImportCounts
	// goverter:enum:transform value
	ContactImportState(source contactimport.State) siteapi.ContactImportState
	// goverter:enum:transform value
	ContactImportDelimiter(source contactimport.Delimiter) siteapi.ContactImportDelimiter
	// goverter:enum:transform value
	ContactImportDateFormat(source csvimport.DateFormat) siteapi.ContactImportDateFormat
	// goverter:enum:transform value
	ContactImportIssueReason(source csvimport.Reason) siteapi.ContactImportIssueReason
	// goverter:enum:transform value
	ContactImportRefusal(source csvimport.Refusal) siteapi.ContactImportRefusal
	// goverter:enum:transform value
	ContactImportRefusalChannel(source channels.Channel) siteapi.ContactImportRefusalChannel
	// A consent or tracking column is never of Telegram (the save refuses it).
	// goverter:enum:transform value
	// goverter:enum:map Telegram @panic
	ContactImportConsentChannel(source channels.Channel) siteapi.ContactImportConsentChannel
	// A preview carries only the address check's traits (ADR 0117): the consent and the
	// two-Contacts warnings are the run's.
	// goverter:enum:transform value
	// goverter:enum:map WarningConsentTimeInFuture @panic
	// goverter:enum:map WarningKeysNameTwoContacts @panic
	ContactImportWarning(source contacts.Warning) siteapi.ContactImportWarning
	// goverter:useEmptyOnNil:slice
	ContactImportPreviewToResource(source contactimports.Preview) siteapi.SiteContactImportPreview
	// goverter:map Refusal | contactImportRefusal
	// goverter:useEmptyOnNil:slice
	ContactImportPreviewRowToResource(source contactimports.PreviewRow) siteapi.ContactImportPreviewRow
	ContactImportIssueToResource(source csvimport.Issue) siteapi.ContactImportIssue
	// A link to download a file (downloadlinks): its path, which the handler builds, and its expiry.
	// goverter:map . URL | contactImportReportURL
	ContactImportReportLink(source contactimports.ReportLink) siteapi.DownloadLink
	// goverter:useEmptyOnNil:slice
	CustomFieldsToResources(source []*ent.CustomField) []siteapi.SiteCustomFieldResource
	StartLinkToResource(source bindings.StartLink) siteapi.TelegramStartLinkResource
	// goverter:map . URL | contactCsvExportURL
	ContactCsvExportLink(source contactsexport.Link) *siteapi.SiteContactCsvExportLink
	BindingToResource(source bindings.Chat) siteapi.SiteBindingResource
	// goverter:useEmptyOnNil:slice
	BindingsToResources(source []bindings.Chat) []siteapi.SiteBindingResource
	SmsConsentToResource(source confirmations.ContactSMS) siteapi.SmsConsent
	// The Engagement tracking consent of a Destination (ADR 0120), and what an observation did.
	EngagementTrackingConsentToResource(source engagementtracking.Consent) siteapi.EngagementTrackingConsent
	ContactEngagementTrackingToResource(source engagementtracking.ContactConsents) siteapi.ContactEngagementTracking
	EngagementTrackingRecordedToResource(source engagementtracking.Recorded) siteapi.EngagementTrackingObservationResult
	// goverter:enum:transform value
	EngagementTrackingChannelToResource(source channels.Channel) siteapi.EngagementTrackingChannel
	// goverter:enum:transform value
	EngagementTrackingReadingToResource(source engagementtracking.Reading) siteapi.EngagementTrackingReading
	// goverter:enum:transform value
	EngagementTrackingProvenanceToResource(source engagementtrackingconsent.Provenance) siteapi.EngagementTrackingProvenance
	// goverter:enum:transform value
	EngagementTrackingDecisionToResource(source engagementtracking.Decision) siteapi.EngagementTrackingDecision
	// goverter:enum:transform value
	EngagementTrackingOutcomeToResource(source engagementtracking.Outcome) siteapi.EngagementTrackingOutcome
	TestSendResultToResource(source outboundmessages.Result) siteapi.SiteTestSendBroadcastResult
	OutsideWindowToResource(source broadcasts.OutsideWindow) siteapi.SiteBroadcastOutsideWindowResult
	SmsLengthLimitToResource(source smsrender.Limit) siteapi.SiteSmsLengthLimit
	SmsOptOutToResource(source *confirmations.SMSOptOut) siteapi.SmsOptOut
	// goverter:enum:transform value
	OptOutViaToResource(source unsubscribe.Via) siteapi.OptOutVia
	// goverter:map CreatedAt ConfirmedAt
	SmsConfirmationToResource(source *ent.Confirmation) siteapi.SmsConfirmation
	// goverter:enum:transform value
	ConfirmationProvenanceToResource(source confirmation.Provenance) siteapi.ConfirmationProvenance
	TagToResource(source *ent.Tag) siteapi.SiteTagResource
	// goverter:useEmptyOnNil:slice
	TagsToResources(source []*ent.Tag) []siteapi.SiteTagResource

	// The transactional send history is the Outbound messages of kind transactional
	// (ADR 0015). A message skipped by Send-eligibility is shown as "suppressed" (the
	// only reason a transactional send is skipped); the reason column is the error
	// text only for a failed send.
	// goverter:map . Status | transactionalStatus
	// goverter:map . Error | transactionalError
	// goverter:map TemplateID TemplateId
	TransactionalMessageToResource(source *ent.OutboundMessage) siteapi.SiteTransactionalEmailResource
	// goverter:useEmptyOnNil:slice
	TransactionalMessagesToResources(source []*ent.OutboundMessage) []siteapi.SiteTransactionalEmailResource

	// A Captured message names its From as the contract does, and the nullable parts as null.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToResource(source *ent.CapturedMessage) siteapi.SiteCapturedMessageResource
	// The list leaves the content out.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToSummary(source *ent.CapturedMessage) siteapi.SiteCapturedMessageSummary
	// goverter:useEmptyOnNil:slice
	CapturedMessagesToSummaries(source []*ent.CapturedMessage) []siteapi.SiteCapturedMessageSummary

	// The member's identity comes from the loaded User edge.
	// goverter:map Edges.User.Email Email
	// goverter:map Edges.User.Name Name
	MembershipToResource(source *ent.Membership) siteapi.SiteMembershipResource
	// goverter:useEmptyOnNil:slice
	MembershipsToResources(source []*ent.Membership) []siteapi.SiteMembershipResource

	// goverter:map . InvitedByEmail | inviterEmail
	// goverter:map Email | pendingEmail
	InvitationToResource(source *ent.Invitation) siteapi.SiteInvitationResource
	// goverter:useEmptyOnNil:slice
	InvitationsToResources(source []*ent.Invitation) []siteapi.SiteInvitationResource
	// What the public accept page shows of a pending invite: the Workspace it joins is the
	// loaded edge.
	// goverter:map Edges.Workspace.Name WorkspaceName
	// goverter:map Email | pendingEmail
	InvitationToLookupResult(source *ent.Invitation) *siteapi.SiteInvitationLookupResult

	// The live version and the draft are the edges the rules module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) (siteapi.SiteRuleResource, error)
	// goverter:useEmptyOnNil:slice
	RulesToResources(source []*ent.Rule) ([]siteapi.SiteRuleResource, error)

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) (siteapi.RuleVersion, error)

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) siteapi.RuleFiringResource
	// goverter:useEmptyOnNil:slice
	RuleFiringsToResources(source []*ent.RuleFiring) []siteapi.RuleFiringResource

	RuleDryRunToResource(source rules.DryRun) siteapi.RuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	// goverter:map Occurrence | enrollmentOccurrence
	// goverter:map . Awaited | enrollmentAwaited
	// goverter:map . HTTP | enrollmentHTTP
	EnrollmentToResource(source *ent.AutomationRun) siteapi.EnrollmentResource
	// goverter:map Properties | awaitedProperties
	KeptEventToResource(source automations.KeptEvent) siteapi.KeptEvent
	// goverter:useEmptyOnNil:slice
	KeptEventsToResources(source []automations.KeptEvent) []siteapi.KeptEvent
	// goverter:map Body | keptBody
	// goverter:map Dropped | keptDropped
	KeptResponseToResource(source automations.KeptResponse) siteapi.KeptResponse
	// goverter:useEmptyOnNil:slice
	KeptResponsesToResources(source []automations.KeptResponse) []siteapi.KeptResponse
	// goverter:enum:transform value
	BodyDroppedToResource(source automationstep.BodyDropped) siteapi.HttpBodyDropped
	// The store names the triggering Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	SkippedEntryToResource(source *ent.AutomationSkippedEntry) siteapi.SkippedEntryResource
	// goverter:useEmptyOnNil:slice
	EnrollmentsToResources(source []*ent.AutomationRun) []siteapi.EnrollmentResource

	// The deletion window: the date the Workspace's data will be erased.
	// goverter:map PublicID ID
	// goverter:map DiscardedAt EraseAt | eraseAt
	// Whether the caller is its only Owner comes from the Memberships, which the mapper
	// cannot see: the handler attaches it.
	// goverter:ignore OnlyOwner
	DeletedWorkspaceToResource(source *ent.Workspace) siteapi.SiteDeletedWorkspaceResource
	// goverter:map DomainJoinEnabled Enabled
	DomainJoinToResource(source *ent.Workspace) *siteapi.SiteDomainJoinResource
	// The Workspace a User who just proved the address lands in.
	// goverter:map PublicID LandingWorkspaceId
	VerifyEmailResultToResource(source *ent.Workspace) siteapi.SiteVerifyEmailResult

	// goverter:map CreatedAt Since
	HoldToResource(source *ent.Hold) siteapi.SiteWorkspaceHold
	// goverter:useEmptyOnNil:slice
	HoldsToResources(source []*ent.Hold) []siteapi.SiteWorkspaceHold

	// goverter:map . Record | joinDomainRecord
	// goverter:map . Verified | joinDomainVerified
	JoinDomainToResource(source *ent.JoinDomain) siteapi.SiteJoinDomainResource
	// goverter:useEmptyOnNil:slice
	JoinDomainsToResources(source []*ent.JoinDomain) []siteapi.SiteJoinDomainResource

	// The DNS records are derived by the sendingdomains module and the envelopes read by its
	// checks, which the mapper cannot see: the handler attaches them. The private key is never
	// exposed.
	// goverter:ignore DkimRecord
	// goverter:ignore DmarcRecord
	// goverter:ignore Envelopes
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) siteapi.SendingDomainResource
	// goverter:useEmptyOnNil:slice
	SendingDomainEnvelopesToResource(source []sendingdomains.Envelope) []siteapi.SendingDomainEnvelope
	// goverter:map MailFrom | envelopeMailFrom
	// goverter:map PermissionNeeded | envelopePermission
	// goverter:map CheckedAt | envelopeCheckedAt
	SendingDomainEnvelopeToResource(source sendingdomains.Envelope) siteapi.SendingDomainEnvelope
	// goverter:map OnMXFailure OnMxFailure
	SendingDomainMailFromToResource(source messaging.MailFrom) siteapi.SendingDomainMailFrom
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) siteapi.DnsRecord

	// The CNAME target is the module's, which the mapper cannot see: the handler attaches it.
	// goverter:ignore Record OwnershipRecord
	// goverter:map CheckedAt LastCheckedAt
	TrackingDomainToResource(source *ent.TrackingDomain) siteapi.TrackingDomainResource
	// goverter:map . Type | cnameRecordType
	CnameRecordToResource(source trackingdomains.Record) siteapi.CnameRecord
	// goverter:map . Type | ownershipRecordType
	OwnershipRecordToResource(source trackingdomains.OwnershipRecord) siteapi.DnsRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkits.Kit) siteapi.BrandKitResource

	// The Brand kit a public page is in (the confirmation page, ADR 0013), as it stands.
	PublicBrandToResource(source brandkits.Kit) siteapi.SitePublicBrand
	ConfirmationPageToResource(source confirmationrequests.Page) siteapi.SiteConfirmationPage

	// goverter:map . Actor
	// goverter:map . Target
	// goverter:map Diff Diff | auditDiff
	AuditEntryToResource(source auditlog.View) siteapi.AuditEntryResource
	// goverter:useEmptyOnNil:slice
	AuditEntriesToResources(source []auditlog.View) []siteapi.AuditEntryResource
	// An entry's actor and target are columns of the entry, nested in the contract.
	// goverter:map ActorKind Kind
	// goverter:map ActorID ID
	// goverter:map ActorName Name
	AuditActorToResource(source auditlog.View) siteapi.AuditActor
	// goverter:map TargetType Type
	// goverter:map TargetID ID
	// goverter:map TargetName Name
	AuditTargetToResource(source auditlog.View) siteapi.AuditTarget
	// goverter:useEmptyOnNil:slice
	AuditFacetsToResource(source auditlog.Facets) siteapi.SiteAuditFacets
	AuditFacetActorToResource(source auditlog.Actor) siteapi.AuditActor

	// The signing secret is decrypted by the webhooks module, which the mapper cannot see:
	// the handler attaches it. The failure state is derived from the failure columns, and an
	// endpoint with no event filter shows an empty list, never null.
	// goverter:ignore Secret
	// goverter:useEmptyOnNil:slice
	// goverter:map . Failure | webhookFailure
	// goverter:map . Enabled | webhookEnabled
	WebhookEndpointToResource(source *ent.WebhookEndpoint) siteapi.SiteWebhookEndpointResource

	// The Data export members (ADR 0021): the contract's typed projection of the
	// stored rows, so the ent JSON tags are never the public format.
	ContactToExport(source *ent.Contact) siteapi.ContactExportContact
	TagToExport(source *ent.Tag) siteapi.ContactExportTag
	VisitorToExport(source *ent.Visitor) siteapi.ContactExportVisitor
	// goverter:map . Properties | eventProperties
	EventToExport(source *ent.Event) siteapi.ContactExportEvent
	UnsubscribeToExport(source *ent.Unsubscribe) siteapi.ContactExportUnsubscribe
	SuppressionToExport(source *ent.Suppression) siteapi.ContactExportSuppression
	ConfirmationToExport(source *ent.Confirmation) siteapi.ContactExportConfirmation
	OutboundMessageToExport(source *ent.OutboundMessage) siteapi.ContactExportOutboundMessage
	BroadcastRecipientToExport(source *ent.BroadcastRecipient) siteapi.ContactExportBroadcastRecipient

	// The Sign-up items in effect and their Legal documents (ADR 0062): the contract calls the
	// current version's label the item's version.
	// goverter:map Label Version
	SignupItemToSite(source signupitems.Document) siteapi.SiteSignupItem
	SignupItemsToSite(source []signupitems.Document) []siteapi.SiteSignupItem
	// goverter:map Label Version
	LegalDocumentToSite(source signupitems.Document) siteapi.SiteLegalDocument
	// The optional consents of account settings with the User's current answer (ADR 0062).
	OptionalConsentToSite(source consentrecords.OptionalConsent) siteapi.SiteOptionalConsent
	OptionalConsentsToSite(source []consentrecords.OptionalConsent) []siteapi.SiteOptionalConsent
	// The banner of a newer release, the one part of the Instance a Workspace's owner sees (ADR 0032).
	InstanceReleaseToResource(source instancestates.Release) siteapi.SiteInstanceRelease

	// Pages: a list response is the module's pagination.Page mapped whole, its items through
	// the list methods above. Items the handler maps itself (an extra per item) arrive as a
	// page of resources (Page.Map), an unpaginated list as pagination.Whole.
	// goverter:useEmptyOnNil:slice
	ContactsListPage(source pagination.Page[*ent.Contact]) *siteapi.SiteContactsListOK
	// goverter:useEmptyOnNil:slice
	SegmentsListPage(source pagination.Page[*ent.Segment]) (*siteapi.SiteSegmentsListOK, error)
	// goverter:useEmptyOnNil:slice
	CustomFieldsListPage(source pagination.Page[*ent.CustomField]) *siteapi.SiteCustomFieldsListOK
	// goverter:useEmptyOnNil:slice
	TagsListPage(source pagination.Page[*ent.Tag]) *siteapi.SiteTagsListOK
	// goverter:useEmptyOnNil:slice
	TagsListForContactPage(source pagination.Page[*ent.Tag]) *siteapi.SiteTagsListForContactOK
	// goverter:useEmptyOnNil:slice
	TransactionalEmailsListPage(source pagination.Page[*ent.OutboundMessage]) *siteapi.SiteTransactionalEmailsListOK
	// goverter:useEmptyOnNil:slice
	CapturedMessagesListPage(source pagination.Page[*ent.CapturedMessage]) *siteapi.SiteCapturedMessagesListOK
	// goverter:useEmptyOnNil:slice
	SkippedEntriesListPage(source pagination.Page[*ent.AutomationSkippedEntry]) *siteapi.SiteSkippedEntriesListOK
	// goverter:useEmptyOnNil:slice
	EnrollmentsListPage(source pagination.Page[*ent.AutomationRun]) *siteapi.SiteEnrollmentsListOK
	// goverter:useEmptyOnNil:slice
	RulesListPage(source pagination.Page[*ent.Rule]) (*siteapi.SiteRulesListOK, error)
	// goverter:useEmptyOnNil:slice
	RuleFiringsListPage(source pagination.Page[*ent.RuleFiring]) *siteapi.SiteRuleFiringsListOK
	// goverter:useEmptyOnNil:slice
	SuppressionsListPage(source pagination.Page[*ent.Suppression]) *siteapi.SiteSuppressionsListOK
	// goverter:useEmptyOnNil:slice
	AutomationsListPage(source pagination.Page[automations.WithRevenue]) (*siteapi.SiteAutomationsListOK, error)
	// goverter:useEmptyOnNil:slice
	EventsListPage(source pagination.Page[*ent.Event]) *siteapi.SiteEventsListOK
	// goverter:useEmptyOnNil:slice
	TemplatesListPage(source pagination.Page[*ent.EmailTemplate]) *siteapi.SiteTemplatesListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	WebhooksListPage(source pagination.Page[siteapi.SiteWebhookEndpointResource]) *siteapi.SiteWebhooksListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	SendingDomainsListPage(source pagination.Page[siteapi.SendingDomainResource]) *siteapi.SiteSendingDomainsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	BroadcastsListPage(source pagination.Page[siteapi.SiteBroadcastResource]) *siteapi.SiteBroadcastsListOK

	SetupChecklistToResource(source setupchecklist.Checklist) siteapi.SiteSetupProgress

	// The Permissions a Role holds, and the one each requested OAuth scope needs.
	// goverter:useEmptyOnNil:slice
	PermissionsToResources(source []memberships.Permission) []siteapi.Permission
	// goverter:useEmptyOnNil:map
	RequiredPermissionsToResource(source map[string]memberships.Permission) siteapi.SiteOAuthAuthorizationRequestRequiredPermissions
	// The scopes a User whose Role it is may give a Personal token.
	// goverter:useEmptyOnNil:slice
	PersonalScopesToResources(source []tokenscope.PersonalScope) []siteapi.PersonalScope

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map ReportPeriod7Days AutomationReportPeriod7d
	// goverter:enum:map ReportPeriod14Days AutomationReportPeriod14d
	// goverter:enum:map ReportPeriod30Days AutomationReportPeriod30d
	// goverter:enum:map ReportPeriod90Days AutomationReportPeriod90d
	AutomationReportPeriodToResource(source automations.ReportPeriod) siteapi.AutomationReportPeriod
	// goverter:enum:transform value
	AbuseMetricToResource(source abusewarning.Metric) siteapi.AbuseMetric
	// goverter:enum:transform value
	AttentionKindToResource(source needsattentionreport.Kind) siteapi.SiteAttentionKind
	// goverter:enum:transform value
	AttentionSeverityToResource(source needsattentionreport.Severity) siteapi.SiteAttentionSeverity
	// goverter:enum:transform value
	AnalyticsOwnerKindToResource(source linkclick.OwnerKind) siteapi.SiteAnalyticsOwnerKind
	// goverter:enum:transform value
	AutomationStatusToResource(source automation.Status) siteapi.AutomationStatus
	// goverter:enum:transform value
	BroadcastStatusToResource(source broadcast.Status) siteapi.BroadcastStatus
	// goverter:enum:transform value
	BroadcastChannelToResource(source channels.Channel) siteapi.BroadcastChannel
	// goverter:enum:transform value
	CapturedMessageSurfaceToResource(source capturedmessage.Surface) siteapi.CapturedMessageSurface
	// goverter:enum:transform value
	ContactExportSourceToResource(source contact.Source) siteapi.ContactExportSource
	// goverter:enum:transform value
	HoldReasonToResource(source holdreason.Reason) siteapi.HoldReason
	// goverter:enum:transform value
	CustomFieldTypeToResource(source customfield.Type) siteapi.CustomFieldType
	// goverter:enum:transform value
	EnrollmentFailureKindToResource(source automationrun.FailureKind) siteapi.EnrollmentFailureKind
	// goverter:enum:transform value
	EnrollmentStatusToResource(source automationrun.Status) siteapi.EnrollmentStatus
	// goverter:enum:transform value
	SkippedEntryReasonToResource(source automationskippedentry.Reason) siteapi.SkippedEntryReason
	// goverter:enum:transform value
	IntegrationChannelToResource(source channels.Channel) siteapi.IntegrationChannel
	// goverter:enum:transform value
	IntegrationProviderToResource(source providers.Provider) siteapi.IntegrationProvider
	// goverter:enum:transform value
	SendingDomainEnvelopeStateToResource(source sendingdomains.EnvelopeState) siteapi.SendingDomainEnvelopeState
	// goverter:enum:transform value
	MailFromStatusToResource(source providers.MailFromStatus) siteapi.MailFromStatus
	// goverter:enum:transform value
	MxFailureBehaviorToResource(source providers.MXFailureBehavior) siteapi.MxFailureBehavior
	// goverter:enum:transform value
	InvitableRoleToResource(source invitation.Role) siteapi.SiteInvitableRole
	// goverter:enum:transform value
	LintCodeToResource(source emailrender.Code) siteapi.LintCode
	// goverter:enum:transform value
	MembershipRoleToResource(source membership.Role) siteapi.SiteMembershipRole
	// goverter:enum:transform value
	MembershipSourceToResource(source membership.Source) siteapi.SiteMembershipSource
	// A User's answer is never `shown`, which only a notice is (ADR 0062).
	// goverter:enum:transform value
	// goverter:enum:map DecisionShown @panic
	OptionalConsentDecisionToResource(source consentrecord.Decision) siteapi.SiteOptionalConsentDecision
	// goverter:enum:transform value
	PermissionToResource(source memberships.Permission) siteapi.Permission
	// goverter:enum:transform value
	PersonalScopeToResource(source tokenscope.PersonalScope) siteapi.PersonalScope
	// goverter:enum:transform value
	ProviderLimitSourceToResource(source providerlimit.Source) siteapi.ProviderLimitSource
	// goverter:enum:transform value
	ProviderLimitWarningToResource(source providerlimit.Warning) siteapi.ProviderLimitWarning
	// goverter:enum:transform value
	RuleActionStatusToResource(source rules.Outcome) siteapi.RuleActionStatus
	// goverter:enum:transform value
	RuleActionTypeToResource(source ruleaction.Type) siteapi.RuleActionType
	// goverter:enum:transform value
	RuleDryRunOutcomeToResource(source rules.DryRunOutcome) siteapi.RuleDryRunOutcome
	// goverter:enum:transform value
	RuleStatusToResource(source rule.Status) siteapi.RuleStatus
	// goverter:enum:transform value
	SetupStepStatusToResource(source setupchecklist.Status) siteapi.SiteSetupStepStatus
	// goverter:enum:transform value
	SignupItemKindToResource(source signupitem.Kind) siteapi.SignupItemKind
	// goverter:enum:transform value
	SuppressionChannelToResource(source channels.Channel) siteapi.SuppressionChannel
	// goverter:enum:transform value
	SuppressionReasonToResource(source suppression.Reason) siteapi.SuppressionReason
	// goverter:enum:transform value
	TrackingReasonToResource(source trackingdomain.Reason) siteapi.TrackingReason
	// goverter:enum:transform value
	SkippedReasonToResource(source skipreason.Reason) siteapi.SkippedReason
	// goverter:enum:transform value
	DeferralCauseToResource(source deferralcause.Cause) siteapi.DeferralCause
	// goverter:enum:transform value
	TrackingStateToResource(source trackingdomain.State) siteapi.TrackingState
	// goverter:enum:transform value
	TrackingDomainRoleToResource(source trackingdomain.Role) siteapi.TrackingDomainRole
	// goverter:enum:transform value
	WebhookDisabledReasonToResource(source webhookendpoint.DisabledReason) siteapi.WebhookDisabledReason
	// What a delivery failed with: the kind a test send answers and a Webhook endpoint keeps.
	// goverter:enum:transform value
	WebhookErrorKindToResource(source webhook.ErrorKind) siteapi.WebhookErrorKind
}

// ExportMapper adapts the Converter to dataexport.Mapper.
type ExportMapper struct{ Converter }

func (m ExportMapper) Contact(r *ent.Contact) json.Marshaler { return new(m.ContactToExport(r)) }
func (m ExportMapper) Tag(r *ent.Tag) json.Marshaler         { return new(m.TagToExport(r)) }
func (m ExportMapper) Visitor(r *ent.Visitor) json.Marshaler { return new(m.VisitorToExport(r)) }
func (m ExportMapper) Event(r *ent.Event) json.Marshaler     { return new(m.EventToExport(r)) }
func (m ExportMapper) Unsubscribe(r *ent.Unsubscribe) json.Marshaler {
	return new(m.UnsubscribeToExport(r))
}
func (m ExportMapper) Suppression(r *ent.Suppression) json.Marshaler {
	return new(m.SuppressionToExport(r))
}
func (m ExportMapper) Confirmation(r *ent.Confirmation) json.Marshaler {
	return new(m.ConfirmationToExport(r))
}
func (m ExportMapper) OutboundMessage(r *ent.OutboundMessage) json.Marshaler {
	return new(m.OutboundMessageToExport(r))
}
func (m ExportMapper) BroadcastRecipient(r *ent.BroadcastRecipient) json.Marshaler {
	return new(m.BroadcastRecipientToExport(r))
}

// providerDisplayName is what a sign-in provider is called to a person.
func providerDisplayName(provider string) string { return signinprovider.DisplayName(provider) }

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

// maxPerDay narrows a rolling 24-hour limit to the contract's bounded int32.
func maxPerDay(n int) siteapi.MaxPerDay { return siteapi.MaxPerDay(narrow.Int32(n)) }

// nilString renders an optional value: null when there is none.
func nilString(v *string) siteapi.NilString {
	return convert.Nil[siteapi.NilString](v)
}

// nilEmailAddress renders an optional address: null when there is none.
func nilEmailAddress(v *string) siteapi.NilEmailAddress {
	return convert.Nil[siteapi.NilEmailAddress]((*siteapi.EmailAddress)(v))
}

// nilEntityID renders an optional reference: null when there is none.
func nilEntityID(v *int64) siteapi.NilEntityId {
	return convert.Nil[siteapi.NilEntityId]((*siteapi.EntityId)(v))
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) siteapi.NilInt32 {
	return convert.Nil[siteapi.NilInt32](convert.Int32Ptr(v))
}

// providerLimitSource is null exactly when the window has no limit (its source is empty).
func providerLimitSource(c Converter, v providerlimit.Source) siteapi.NilProviderLimitSource {
	if v == "" {
		return siteapi.NilProviderLimitSource{Null: true}
	}
	return siteapi.NewNilProviderLimitSource(c.ProviderLimitSourceToResource(v))
}

// optNilSendCap is null exactly when no Send cap applies.
func optNilSendCap(c Converter, v *providerlimit.SendCapStatus) siteapi.OptNilSendCap {
	return convert.NilVia[siteapi.OptNilSendCap](v, c.SendCapToResource)
}

// optNilSmsConfirmation is absent exactly when the phone has no SMS Confirmation.
func optNilSmsConfirmation(c Converter, v *ent.Confirmation) siteapi.OptNilSmsConfirmation {
	if v == nil {
		return siteapi.OptNilSmsConfirmation{}
	}
	return siteapi.NewOptNilSmsConfirmation(c.SmsConfirmationToResource(v))
}

// optNilSmsOptOut is absent exactly when the phone has no SMS opt-out.
func optNilSmsOptOut(c Converter, v *confirmations.SMSOptOut) siteapi.OptNilSmsOptOut {
	if v == nil {
		return siteapi.OptNilSmsOptOut{}
	}
	return siteapi.NewOptNilSmsOptOut(c.SmsOptOutToResource(v))
}

// broadcastLimit is null exactly when nothing limits the Broadcast (its limit is empty).
func broadcastLimit(c Converter, v deferralcause.Cause) siteapi.NilSiteBroadcastLimit {
	if v == "" {
		return siteapi.NilSiteBroadcastLimit{Null: true}
	}
	return siteapi.NewNilSiteBroadcastLimit(c.BroadcastLimitToResource(v))
}

// nilFloat32 renders a figure a Channel may not report: null when it does not.
func nilFloat32(v *float32) siteapi.NilFloat32 {
	return convert.Nil[siteapi.NilFloat32](v)
}

// broadcastContent is a Broadcast's content in the arm of its Channel: a oneOf is beyond
// goverter, so only the switch is written here.
func broadcastContent(c Converter, b *ent.Broadcast) siteapi.BroadcastContent {
	switch b.Channel {
	case channels.Email:
		return siteapi.BroadcastContent{OneOf: siteapi.NewEmailBroadcastContentBroadcastContentSum(c.EmailBroadcastContentToResource(b))}
	case channels.Telegram:
		return siteapi.BroadcastContent{OneOf: siteapi.NewTelegramBroadcastContentBroadcastContentSum(c.TelegramBroadcastContentToResource(b))}
	case channels.SMS:
		return siteapi.BroadcastContent{OneOf: siteapi.NewSmsBroadcastContentBroadcastContentSum(c.SmsBroadcastContentToResource(b))}
	}
	return siteapi.BroadcastContent{}
}

// smsContentChannel is the discriminator of SMS's content arm.
func smsContentChannel(channels.Channel) siteapi.SmsBroadcastContentChannel {
	return siteapi.SmsBroadcastContentChannelSMS
}

// telegramContentChannel is the discriminator of Telegram's content arm.
func telegramContentChannel(channels.Channel) siteapi.TelegramBroadcastContentChannel {
	return siteapi.TelegramBroadcastContentChannelTelegram
}

// emailContentChannel is the discriminator of email's content arm.
func emailContentChannel(channels.Channel) siteapi.EmailBroadcastContentChannel {
	return siteapi.EmailBroadcastContentChannelEmail
}

// jsonObject renders a stored JSON object (custom fields, event properties): absent when it has
// no keys.
// eventProperties is an Event's properties as a client sees them, a System event's keys by their
// public form (systemevent.PublicProperties).
func eventProperties(e *ent.Event) siteapi.OptNilJsonObject {
	return jsonObject(systemevent.PublicProperties(e.Action, e.Properties))
}

func jsonObject(m map[string]any) siteapi.OptNilJsonObject {
	return convert.OptObject[siteapi.OptNilJsonObject](convert.RawObject[siteapi.JsonObject](m))
}

// editorDocument is a stored editor document (a Template's, a Broadcast's) as the opaque object of
// the contract; a message written as HTML has none.
func editorDocument(doc json.RawMessage) siteapi.OptNilEditorDocument {
	return convert.OptObject[siteapi.OptNilEditorDocument](convert.DocObject[siteapi.EditorDocument](doc))
}

// emailVerified derives the verified flag from the nullable timestamp.
func emailVerified(t *time.Time) bool {
	return t != nil
}

func timestamp(t time.Time) siteapi.Timestamp {
	return siteapi.Timestamp(t)
}

// transactionalChannel maps the Outbound message channel onto the transactional
// send channel enum (email is the only channel built).
func transactionalChannel(channels.Channel) siteapi.SiteTransactionalEmailChannel {
	return siteapi.SiteTransactionalEmailChannelEmail
}

// transactionalStatus maps an Outbound message's status onto the transactional
// send status enum.
func transactionalStatus(m *ent.OutboundMessage) siteapi.SiteTransactionalEmailStatus {
	switch m.Status {
	case outboundmessage.StatusSent:
		return siteapi.SiteTransactionalEmailStatusSent
	case outboundmessage.StatusSkipped:
		return siteapi.SiteTransactionalEmailStatusSuppressed
	case outboundmessage.StatusFailed:
		return siteapi.SiteTransactionalEmailStatusFailed
	default:
		return siteapi.SiteTransactionalEmailStatusPending
	}
}

// transactionalError exposes the failure text only for a failed send.
func transactionalError(m *ent.OutboundMessage) siteapi.OptNilString {
	if m.Status != outboundmessage.StatusFailed || m.Failure == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*m.Failure)
}

func optNilString(v *string) siteapi.OptNilString {
	return convert.Opt[siteapi.OptNilString](v)
}

// optNilStrings is a list stored as NULL when it has nothing to say (nil), absent then.
func optNilStrings(v []string) siteapi.OptNilStringArray {
	if v == nil {
		return siteapi.OptNilStringArray{}
	}
	return siteapi.NewOptNilStringArray(v)
}

func optNilEmailAddress(v *string) siteapi.OptNilEmailAddress {
	return convert.Opt[siteapi.OptNilEmailAddress]((*siteapi.EmailAddress)(v))
}

// optStepID is a Step id absent when there is none.
func optStepID(v *automationstep.ID) siteapi.OptStepId {
	return convert.Opt[siteapi.OptStepId]((*siteapi.StepId)(v))
}

// optNilStepID is the Step id an Automation message names, null for another kind of message.
func optNilStepID(v *automationstep.ID) siteapi.OptNilString {
	return convert.Opt[siteapi.OptNilString]((*string)(v))
}

// optEntityID is an id absent when there is none.
func optEntityID(v *int64) siteapi.OptEntityId {
	return convert.Opt[siteapi.OptEntityId]((*siteapi.EntityId)(v))
}

func optNilEntityID(v *int64) siteapi.OptNilEntityId {
	return convert.Opt[siteapi.OptNilEntityId]((*siteapi.EntityId)(v))
}

// durationSeconds is a duration in whole seconds, null when there was nothing to measure.
func durationSeconds(d *time.Duration) siteapi.NilInt64 {
	return convert.Nil[siteapi.NilInt64](convert.WholeSeconds(d))
}

// workspaceTimezone is the Workspace time zone, UTC while an Owner has not set one (ADR 0057).
func workspaceTimezone(v *string) siteapi.TimeZoneName {
	return siteapi.TimeZoneName(reportday.Name(v))
}

// nilCurrencyCode is the Reporting currency, null until an Owner or Admin chooses one (ADR 0064).
func nilCurrencyCode(v *string) siteapi.NilCurrencyCode {
	return convert.Nil[siteapi.NilCurrencyCode]((*siteapi.CurrencyCode)(v))
}

// nilTimeZone is the User time zone (ADR 0068), null while the app follows the browser.
func nilTimeZone(v *string) siteapi.NilTimeZoneName {
	return convert.Nil[siteapi.NilTimeZoneName]((*siteapi.TimeZoneName)(v))
}

// nilRegionalFormat is the User's Regional format, null while the app follows the Instance's language.
func nilRegionalFormat(v *string) siteapi.NilRegionalFormat {
	return convert.Nil[siteapi.NilRegionalFormat]((*siteapi.RegionalFormat)(v))
}

func attributionWindowDays(v int) siteapi.AttributionWindowDays {
	return siteapi.AttributionWindowDays(narrow.Int32(v))
}

// nilCountryCode is a country of a sending rule, null for the Channel's own.
func nilCountryCode(v *string) siteapi.NilCountryCode {
	return convert.Nil[siteapi.NilCountryCode]((*siteapi.CountryCode)(v))
}

// nilFrequencyCapMaxMessages is a Frequency cap's maximum, null where the row removes the cap.
func nilFrequencyCapMaxMessages(v *int) siteapi.NilFrequencyCapMaxMessages {
	return convert.NilVia[siteapi.NilFrequencyCapMaxMessages](v, func(n int) siteapi.FrequencyCapMaxMessages {
		return siteapi.FrequencyCapMaxMessages(narrow.Int32(n))
	})
}

// nilFrequencyCapPeriodMinutes is a Frequency cap's period, null where the row removes the cap.
func nilFrequencyCapPeriodMinutes(v *int) siteapi.NilFrequencyCapPeriodMinutes {
	return convert.NilVia[siteapi.NilFrequencyCapPeriodMinutes](v, func(n int) siteapi.FrequencyCapPeriodMinutes {
		return siteapi.FrequencyCapPeriodMinutes(narrow.Int32(n))
	})
}

func minuteOfDay(v int) siteapi.MinuteOfDay { return siteapi.MinuteOfDay(narrow.Int32(v)) }

func optNilTimeZone(v *string) siteapi.OptNilTimeZoneName {
	return convert.Opt[siteapi.OptNilTimeZoneName]((*siteapi.TimeZoneName)(v))
}

func optNilCountry(v *string) siteapi.OptNilCountryCode {
	return convert.Opt[siteapi.OptNilCountryCode]((*siteapi.CountryCode)(v))
}

func optNilLocale(v *string) siteapi.OptNilLocaleTag {
	return convert.Opt[siteapi.OptNilLocaleTag]((*siteapi.LocaleTag)(v))
}

func optNilImageURL(v *string) siteapi.OptNilImageUrl {
	return convert.Opt[siteapi.OptNilImageUrl]((*siteapi.ImageUrl)(v))
}

func optNilDate(v *time.Time) siteapi.OptNilDate { return convert.Opt[siteapi.OptNilDate](v) }

func optNilTimestamp(v *time.Time) siteapi.OptNilTimestamp {
	return convert.Opt[siteapi.OptNilTimestamp]((*siteapi.Timestamp)(v))
}

// nilTimestamp is a required time that may be null (the first Order of a Workspace that sent none).
func nilTimestamp(v *time.Time) siteapi.NilTimestamp {
	return convert.Nil[siteapi.NilTimestamp]((*siteapi.Timestamp)(v))
}

// automationVersionOrNull is a version of the Automation, or null when it has none.
func automationVersionOrNull(c Converter, v *ent.AutomationVersion) (siteapi.NilAutomationVersion, error) {
	if v == nil {
		return siteapi.NilAutomationVersion{Null: true}, nil
	}
	res, err := c.AutomationVersionToResource(v)
	if err != nil {
		return siteapi.NilAutomationVersion{}, err
	}
	return siteapi.NewNilAutomationVersion(res), nil
}

// automationPath maps a version's steps into the typed path DTO, through the automations module's
// masked view of each step.
func automationPath(c Converter, steps automationstep.Path) (siteapi.AutomationPath, error) {
	view, err := automations.View(steps)
	if err != nil {
		return siteapi.AutomationPath{}, err
	}
	return c.AutomationPathToResource(view)
}

// automationStepViews wraps each Step's view of the root path in the arm of its kind (stepSum).
func automationStepViews(c Converter, views []automations.StepView) ([]siteapi.AutomationStep, error) {
	out := make([]siteapi.AutomationStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, siteapi.AutomationStep{OneOf: sum})
	}
	return out, nil
}

// pathStepViews wraps each Step's view of a fork's path in the arm of its kind (stepSum): the
// contract's union of a fork's path has the arms of the root's.
func pathStepViews(c Converter, views []automations.StepView) ([]siteapi.PathStep, error) {
	out := make([]siteapi.PathStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, siteapi.PathStep{OneOf: c.PathStepSum(sum)})
	}
	return out, nil
}

// splitPathStepViews wraps each Step's view of a Split's path in the arm of its kind (stepSum): the
// contract's union of a Split's path has the arms of the root's.
func splitPathStepViews(c Converter, views []automations.StepView) ([]siteapi.SplitPathStep, error) {
	out := make([]siteapi.SplitPathStep, 0, len(views))
	for _, v := range views {
		sum, err := stepSum(c, v)
		if err != nil {
			return nil, err
		}
		out = append(out, siteapi.SplitPathStep{OneOf: c.SplitPathStepSum(sum)})
	}
	return out, nil
}

// stepSum is the arm of the Step's view: a oneOf is beyond goverter, so only the switch is written
// here. A view with no arm fails the response, never shown as an empty Step.
func stepSum(c Converter, v automations.StepView) (siteapi.AutomationStepSum, error) {
	switch v := v.(type) {
	case automations.EmailView:
		return siteapi.NewEmailStepAutomationStepSum(c.EmailStepToResource(v)), nil
	case automations.TelegramView:
		return siteapi.NewTelegramStepAutomationStepSum(c.TelegramStepToResource(v)), nil
	case automations.SMSView:
		return siteapi.NewSmsStepAutomationStepSum(c.SmsStepToResource(v)), nil
	case automations.WaitView:
		return siteapi.NewWaitStepAutomationStepSum(c.WaitStepToResource(v)), nil
	case automations.WaitUntilView:
		w, err := c.WaitUntilStepToResource(v)
		if err != nil {
			return siteapi.AutomationStepSum{}, err
		}
		return siteapi.NewWaitUntilStepAutomationStepSum(w), nil
	case automations.TagView:
		// The module shows both tag kinds as one view; the union has an arm for each.
		switch v.Type {
		case automationstep.KindApplyTag:
			return siteapi.NewApplyTagStepAutomationStepSum(c.ApplyTagStepToResource(v)), nil
		case automationstep.KindRemoveTag:
			return siteapi.NewRemoveTagStepAutomationStepSum(c.RemoveTagStepToResource(v)), nil
		default:
			return siteapi.AutomationStepSum{}, fmt.Errorf("a tag step view of kind %q has no arm", v.Type)
		}
	case automations.HTTPView:
		return siteapi.NewHttpStepAutomationStepSum(c.HttpStepToResource(v)), nil
	case automations.EndView:
		return siteapi.NewEndStepAutomationStepSum(c.EndStepToResource(v)), nil
	case automations.BranchView:
		b, err := c.BranchStepToResource(v)
		if err != nil {
			return siteapi.AutomationStepSum{}, err
		}
		return siteapi.NewBranchStepAutomationStepSum(b), nil
	case automations.SplitView:
		sp, err := c.SplitStepToResource(v)
		if err != nil {
			return siteapi.AutomationStepSum{}, err
		}
		return siteapi.NewSplitStepAutomationStepSum(sp), nil
	case automations.WaitForEventView:
		w, err := c.WaitForEventStepToResource(v)
		if err != nil {
			return siteapi.AutomationStepSum{}, err
		}
		return siteapi.NewWaitForEventStepAutomationStepSum(w), nil
	default:
		return siteapi.AutomationStepSum{}, fmt.Errorf("the step view %T has no arm", v)
	}
}

// optAutomationPath is a Branch's unknown path, absent for none.
func optAutomationPath(c Converter, v *automations.PathView) (siteapi.OptAutomationPath, error) {
	if v == nil {
		return siteapi.OptAutomationPath{}, nil
	}
	p, err := c.AutomationPathToResource(*v)
	if err != nil {
		return siteapi.OptAutomationPath{}, err
	}
	return siteapi.NewOptAutomationPath(p), nil
}

// branchCondition maps a Branch path's Condition into the typed DTO.
func branchCondition(g conditiontree.Branch) (siteapi.BranchCondition, error) {
	return convert.Document[siteapi.BranchCondition](g)
}

// The discriminators of the Step arms.
func emailStepType(automationstep.Kind) siteapi.EmailStepType { return siteapi.EmailStepTypeEmail }
func telegramStepType(automationstep.Kind) siteapi.TelegramStepType {
	return siteapi.TelegramStepTypeTelegram
}
func smsStepType(automationstep.Kind) siteapi.SmsStepType   { return siteapi.SmsStepTypeSMS }
func waitStepType(automationstep.Kind) siteapi.WaitStepType { return siteapi.WaitStepTypeWait }
func waitUntilStepType(automationstep.Kind) siteapi.WaitUntilStepType {
	return siteapi.WaitUntilStepTypeWaitUntil
}
func waitUntilTimeOfDayForm(automationstep.TimeOfDay) siteapi.WaitUntilTimeOfDayForm {
	return siteapi.WaitUntilTimeOfDayFormTimeOfDay
}

// waitUntilMoment is the arm of a Wait until's moment: a oneOf is beyond goverter, so only the
// switch is written here. A moment with no arm fails the response.
func waitUntilMoment(c Converter, u automationstep.Until) (siteapi.WaitUntilMoment, error) {
	switch u := u.(type) {
	case automationstep.TimeOfDay:
		return siteapi.WaitUntilMoment{OneOf: siteapi.NewWaitUntilTimeOfDayWaitUntilMomentSum(c.WaitUntilTimeOfDayToResource(u))}, nil
	case automationstep.OnDate:
		return siteapi.WaitUntilMoment{OneOf: siteapi.NewWaitUntilDateWaitUntilMomentSum(c.WaitUntilDateToResource(u.Date))}, nil
	case nil:
		return siteapi.WaitUntilMoment{}, errors.New("a wait until without a moment")
	}
	return siteapi.WaitUntilMoment{}, fmt.Errorf("the moment %T has no arm", u)
}

func applyTagStepType(automationstep.Kind) siteapi.ApplyTagStepType {
	return siteapi.ApplyTagStepTypeApplyTag
}
func removeTagStepType(automationstep.Kind) siteapi.RemoveTagStepType {
	return siteapi.RemoveTagStepTypeRemoveTag
}
func httpStepType(automationstep.Kind) siteapi.HttpStepType { return siteapi.HttpStepTypeHTTP }
func endStepType(automationstep.Kind) siteapi.EndStepType   { return siteapi.EndStepTypeEnd }
func branchStepType(automationstep.Kind) siteapi.BranchStepType {
	return siteapi.BranchStepTypeBranch
}
func splitStepType(automationstep.Kind) siteapi.SplitStepType {
	return siteapi.SplitStepTypeSplit
}
func waitForEventStepType(automationstep.Kind) siteapi.WaitForEventStepType {
	return siteapi.WaitForEventStepTypeWaitForEvent
}
func stepName(name string) siteapi.StepName { return siteapi.StepName(name) }

// waitForEventAwaits is the arm of what a Wait for event waits for: a oneOf is beyond goverter, so
// only the switch is written here. One with no arm fails the response.
func waitForEventAwaits(c Converter, a automationstep.Awaits) (siteapi.WaitForEventAwaits, error) {
	switch a := a.(type) {
	case automationstep.AwaitedEvent:
		ev, err := awaitedEvent(a)
		if err != nil {
			return siteapi.WaitForEventAwaits{}, err
		}
		return siteapi.WaitForEventAwaits{OneOf: siteapi.NewAwaitedEventWaitForEventAwaitsSum(ev)}, nil
	case automationstep.AwaitedMessage:
		m := siteapi.AwaitedMessage{Kind: siteapi.AwaitedMessageKindMessage, Step: siteapi.StepId(a.Step), Engagement: c.MessageEngagementToResource(a.Engagement)}
		return siteapi.WaitForEventAwaits{OneOf: siteapi.NewAwaitedMessageWaitForEventAwaitsSum(m)}, nil
	case nil:
	}
	return siteapi.WaitForEventAwaits{}, errors.New("a wait for event that awaits nothing")
}

// awaitedEvent maps a Wait for event's Event matchers and its Condition into the typed DTO, each
// checked as a stored Condition is (convert.Document).
func awaitedEvent(a automationstep.AwaitedEvent) (siteapi.AwaitedEvent, error) {
	ev := siteapi.AwaitedEvent{Kind: siteapi.AwaitedEventKindEvent, Matchers: make([]siteapi.EventMatcher, len(a.Matchers))}
	for i, m := range a.Matchers {
		matcher, err := eventMatcher(m)
		if err != nil {
			return siteapi.AwaitedEvent{}, err
		}
		ev.Matchers[i] = matcher
	}
	if a.Condition != nil {
		cond, err := convert.Document[siteapi.WaitForEventCondition](*a.Condition)
		if err != nil {
			return siteapi.AwaitedEvent{}, err
		}
		ev.Condition = siteapi.NewOptWaitForEventCondition(cond)
	}
	return ev, nil
}

func optEditorDocument(doc json.RawMessage) siteapi.OptEditorDocument {
	return convert.OptObject[siteapi.OptEditorDocument](convert.DocObject[siteapi.EditorDocument](doc))
}

// segmentCondition maps a Segment's Condition into the typed DTO. A stored Condition is checked when it
// is written, so one that does not decode is a broken row, and it fails the response.
func segmentCondition(g conditiontree.Segment) (siteapi.SegmentCondition, error) {
	return convert.Document[siteapi.SegmentCondition](g)
}

// automationTrigger maps a version's Trigger into the typed DTO by its kind: an Event matcher whose
// filter is checked as a stored Condition is (convert.Document).
func automationTrigger(c Converter, v *ent.AutomationVersion) (siteapi.AutomationTrigger, error) {
	switch t := automations.TriggerOf(v).(type) {
	case automations.DateTrigger:
		return siteapi.AutomationTrigger{OneOf: siteapi.NewDateTriggerAutomationTriggerSum(c.DateTriggerToResource(t.Date))}, nil
	case automations.EventTrigger:
		ev := siteapi.EventTrigger{
			Kind: siteapi.EventTriggerKindEvent, Action: t.Matcher.Action,
			EntryPolicy: c.EntryPolicyToResource(v.EntryPolicy), Cooldown: siteapi.EntryCooldown(narrow.Int32(v.CooldownSeconds)),
			EnrollmentKey: optString(v.EnrollmentKey),
		}
		if t.Matcher.Filter != nil {
			f, err := convert.Document[siteapi.EventFilter](*t.Matcher.Filter)
			if err != nil {
				return siteapi.AutomationTrigger{}, err
			}
			ev.Filter = siteapi.NewOptEventFilter(f)
		}
		return siteapi.AutomationTrigger{OneOf: siteapi.NewEventTriggerAutomationTriggerSum(ev)}, nil
	}
	return siteapi.AutomationTrigger{}, fmt.Errorf("version %d has a trigger of no kind", v.ID)
}

func dateTriggerKind(datetrigger.Date) siteapi.DateTriggerKind { return siteapi.DateTriggerKindDate }
func waitUntilDateForm(datetrigger.Date) siteapi.WaitUntilDateForm {
	return siteapi.WaitUntilDateFormDate
}

// dateOffsetDays is a date's offset in days, within the contract's bound when it was stored.
func dateOffsetDays(days int) siteapi.DateOffsetDays {
	return siteapi.DateOffsetDays(narrow.Int32(days))
}

// dateField maps a date Trigger's field into the typed DTO by its kind. A stored field is checked
// when it is written, so a kind it does not know is the birthday's never.
func dateField(f datetrigger.Field) siteapi.DateField {
	switch f.Kind {
	case datetrigger.CustomField:
		return siteapi.DateField{OneOf: siteapi.NewCustomFieldDateFieldDateFieldSum(siteapi.CustomFieldDateField{
			Kind: siteapi.CustomFieldDateFieldKindCustomField, Key: f.Key,
		})}
	case datetrigger.Birthday:
	}
	return siteapi.DateField{OneOf: siteapi.NewBirthdayDateFieldDateFieldSum(siteapi.BirthdayDateField{Kind: siteapi.BirthdayDateFieldKindBirthday})}
}

// eventMatcher maps an exit or goal event into the typed DTO, its filter checked as a stored
// Condition is (convert.Document).
func eventMatcher(m eventmatcher.Matcher) (siteapi.EventMatcher, error) {
	out := siteapi.EventMatcher{Action: m.Action}
	if m.Filter != nil {
		f, err := convert.Document[siteapi.EventFilter](*m.Filter)
		if err != nil {
			return siteapi.EventMatcher{}, err
		}
		out.Filter = siteapi.NewOptEventFilter(f)
	}
	return out, nil
}

// automationExits maps a version's exit events into the typed DTO; never nil.
func automationExits(v *ent.AutomationVersion) ([]siteapi.EventMatcher, error) {
	exits := automations.ExitsOf(v)
	out := make([]siteapi.EventMatcher, len(exits))
	for i, e := range exits {
		m, err := eventMatcher(e)
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// automationGoal maps a version's goal event into the typed DTO: null when there is none.
func automationGoal(goal *eventmatcher.Matcher) (siteapi.NilEventMatcher, error) {
	if goal == nil {
		return siteapi.NilEventMatcher{Null: true}, nil
	}
	m, err := eventMatcher(*goal)
	if err != nil {
		return siteapi.NilEventMatcher{}, err
	}
	return siteapi.NewNilEventMatcher(m), nil
}

// ruleCondition maps a Rule's Condition into the typed DTO, failing as segmentCondition does.
func ruleCondition(g conditiontree.Rule) (siteapi.RuleCondition, error) {
	return convert.Document[siteapi.RuleCondition](g)
}

func optTimestamp(v *time.Time) siteapi.OptTimestamp {
	return convert.Opt[siteapi.OptTimestamp]((*siteapi.Timestamp)(v))
}

func optInt32(v *int) siteapi.OptInt32 {
	return convert.Opt[siteapi.OptInt32](convert.Int32Ptr(v))
}

func optString(v *string) siteapi.OptString {
	return convert.Opt[siteapi.OptString](v)
}

// optEmail is an address, unset when there is none.
func optEmail(v string) siteapi.OptEmailAddress {
	if v == "" {
		return siteapi.OptEmailAddress{}
	}
	return siteapi.NewOptEmailAddress(siteapi.EmailAddress(v))
}

// optConsentDecision is a User's current answer to an optional consent, absent with none; it is
// never `shown`, which only a notice is.
func optConsentDecision(c Converter, v *consentrecord.Decision) siteapi.OptSiteOptionalConsentDecision {
	return convert.OptVia[siteapi.OptSiteOptionalConsentDecision](v, c.OptionalConsentDecisionToResource)
}

func optTrackingReason(c Converter, v *trackingdomain.Reason) siteapi.OptTrackingReason {
	return convert.OptVia[siteapi.OptTrackingReason](v, c.TrackingReasonToResource)
}

// optSkippedReason is absent on every message but a skipped one.
func optSkippedReason(c Converter, v *skipreason.Reason) siteapi.OptNilSkippedReason {
	return convert.OptVia[siteapi.OptNilSkippedReason](v, c.SkippedReasonToResource)
}

// optDeferralCause is absent on everything that does not wait.
func optDeferralCause(c Converter, v *deferralcause.Cause) siteapi.OptDeferralCause {
	return convert.OptVia[siteapi.OptDeferralCause](v, c.DeferralCauseToResource)
}

// tagNames lists the names of a contact's loaded tags, in the order they were read.
func tagNames(tags []*ent.Tag) []string {
	return lo.Map(tags, func(t *ent.Tag, _ int) string { return t.Name })
}

// timestampOf renders a time the schema keeps nullable but the contract always shows.
func timestampOf(t *time.Time) siteapi.Timestamp {
	if t == nil {
		return siteapi.Timestamp{}
	}
	return siteapi.Timestamp(*t)
}

// eraseAt is the date a discarded Workspace's data will be erased.
func eraseAt(discardedAt *time.Time) siteapi.Timestamp {
	if discardedAt == nil {
		return siteapi.Timestamp{}
	}
	return siteapi.Timestamp(workspaces.EraseDate(*discardedAt))
}

// pendingEmail is a pending Invitation's address; the API lists and looks up pending ones alone,
// and only an accepted one has none.
func pendingEmail(email *string) siteapi.EmailAddress {
	return siteapi.EmailAddress(lo.FromPtr(email))
}

// inviterEmail is the inviting User's address when the inviter edge is loaded.
func inviterEmail(inv ent.Invitation) siteapi.OptNilString {
	if inv.Edges.Inviter == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(inv.Edges.Inviter.Email)
}

// enrollmentOccurrence is the occurrence of a date Trigger the enrollment was made for (ADR 0126):
// its anniversary year, or a once date's one occurrence; absent for one an Event or a rule began.
func enrollmentOccurrence(stored *datetrigger.StoredOccurrence) siteapi.OptEnrollmentOccurrence {
	if stored == nil {
		return siteapi.OptEnrollmentOccurrence{}
	}
	var sum siteapi.EnrollmentOccurrenceSum
	switch o := stored.Occurrence().(type) {
	case datetrigger.Yearly:
		sum = siteapi.NewYearlyEnrollmentOccurrenceEnrollmentOccurrenceSum(siteapi.YearlyEnrollmentOccurrence{
			Recurrence: siteapi.YearlyEnrollmentOccurrenceRecurrenceYearly, Year: count(o.Year),
		})
	case datetrigger.Once:
		sum = siteapi.NewOnceEnrollmentOccurrenceEnrollmentOccurrenceSum(siteapi.OnceEnrollmentOccurrence{
			Recurrence: siteapi.OnceEnrollmentOccurrenceRecurrenceOnce,
		})
	}
	return siteapi.NewOptEnrollmentOccurrence(siteapi.EnrollmentOccurrence{OneOf: sum})
}

// enrollmentAwaited is what the enrollment keeps of the Events its Wait for event Steps arrived on
// (ADR 0136), by name; empty once it ended.
func enrollmentAwaited(c Converter, r ent.AutomationRun) []siteapi.KeptEvent {
	return c.KeptEventsToResources(automations.KeptEvents(&r))
}

// awaitedProperties is a kept Event's properties, an empty object when it kept none.
func awaitedProperties(m map[string]any) siteapi.JsonObject {
	return convert.RawObject[siteapi.JsonObject](m)
}

// enrollmentHTTP is what the enrollment keeps of the responses its HTTP steps received (ADR 0161),
// by name: empty unless it was read with them, and once it ended.
func enrollmentHTTP(c Converter, r ent.AutomationRun) []siteapi.KeptResponse {
	return c.KeptResponsesToResources(automations.KeptResponses(&r))
}

// keptBody is the JSON object a kept response carried, absent when its body was dropped; a kept
// empty object is present and empty.
func keptBody(m map[string]any) siteapi.OptJsonObject {
	if m == nil {
		return siteapi.OptJsonObject{}
	}
	body := convert.RawObject[siteapi.JsonObject](m)
	if body == nil {
		body = siteapi.JsonObject{}
	}
	return siteapi.NewOptJsonObject(body)
}

// keptDropped is why a kept response's body was dropped, absent when it was kept.
func keptDropped(c Converter, d *automationstep.BodyDropped) siteapi.OptHttpBodyDropped {
	return convert.OptVia[siteapi.OptHttpBodyDropped](d, c.BodyDroppedToResource)
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(c Converter, r ent.AutomationRun) siteapi.OptEnrollmentFailure {
	if r.FailureKind == nil {
		return siteapi.OptEnrollmentFailure{}
	}
	return siteapi.NewOptEnrollmentFailure(siteapi.EnrollmentFailure{
		Kind:    c.EnrollmentFailureKindToResource(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

// joinDomainRecord is the TXT record the Owner publishes to prove the domain.
func joinDomainRecord(d ent.JoinDomain) siteapi.DnsRecord {
	rec := joindomains.RecordOf(&d)
	return siteapi.DnsRecord{Type: siteapi.DnsRecordTypeTXT, Host: rec.Host, Value: rec.Value}
}

func joinDomainVerified(d ent.JoinDomain) bool { return joindomains.Verified(&d) }

func dnsRecordType(sendingdomains.Record) siteapi.DnsRecordType {
	return siteapi.DnsRecordTypeTXT
}

func cnameRecordType(trackingdomains.Record) siteapi.CnameRecordType {
	return siteapi.CnameRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkits.Kit) siteapi.OptNilTimestamp {
	if !k.Saved {
		return siteapi.OptNilTimestamp{}
	}
	return siteapi.NewOptNilTimestamp(siteapi.Timestamp(k.UpdatedAt))
}

func auditDiff(d map[string]jx.Raw) siteapi.OptNilJsonObject {
	return convert.OptObject[siteapi.OptNilJsonObject](siteapi.JsonObject(d))
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(c Converter, e ent.WebhookEndpoint) siteapi.OptWebhookFailure {
	h := webhookendpoints.HealthOf(&e)
	if h.ConsecutiveFailures == nil || h.FailingSince == nil || h.LastFailureAt == nil || h.LastErrorKind == nil {
		return siteapi.OptWebhookFailure{}
	}
	return siteapi.NewOptWebhookFailure(siteapi.WebhookFailure{
		ConsecutiveFailures: narrow.Int32(*h.ConsecutiveFailures),
		Since:               siteapi.Timestamp(*h.FailingSince),
		LastFailureAt:       siteapi.Timestamp(*h.LastFailureAt),
		Kind:                c.WebhookErrorKindToResource(*h.LastErrorKind),
		Status:              optInt32(h.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(c Converter, v *webhookendpoint.DisabledReason) siteapi.OptWebhookDisabledReason {
	return convert.OptVia[siteapi.OptWebhookDisabledReason](v, c.WebhookDisabledReasonToResource)
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) siteapi.OptNilString {
	return convert.OptNonZero[siteapi.OptNilString](s)
}

// omitEmptyConfigurationSet is an SES Integration's configuration set, absent when it names none.
func omitEmptyConfigurationSet(s string) siteapi.OptNilSesConfigurationSetName {
	return convert.OptNonZero[siteapi.OptNilSesConfigurationSetName](siteapi.SesConfigurationSetName(s))
}

func smtpConfigKind(integrations.RedactedSMTP) siteapi.SmtpConfigKind {
	return siteapi.SmtpConfigKindSMTP
}

func sesConfigKind(integrations.RedactedSES) siteapi.SesConfigKind {
	return siteapi.SesConfigKindSes
}

func twilioConfigKind(integrations.RedactedTwilio) siteapi.TwilioConfigKind {
	return siteapi.TwilioConfigKindTwilio
}

// twilioAPIKeySID is the API key messages are sent with, null when they are sent with the Auth Token.
func twilioAPIKeySID(c integrations.RedactedTwilio) siteapi.NilString {
	if c.APIKeySID == "" {
		return siteapi.NilString{Null: true}
	}
	return siteapi.NewNilString(c.APIKeySID)
}

func telegramConfigKind(integrations.RedactedTelegram) siteapi.TelegramConfigKind {
	return siteapi.TelegramConfigKindTelegram
}

// sendingDomainVerified derives the verified flag from the verified and lost moments.
func sendingDomainVerified(d ent.SendingDomain) bool { return dkimstate.Is(&d) }

// webhookEnabled derives the enabled flag: an endpoint is enabled until it has a disabled moment.
func webhookEnabled(e ent.WebhookEndpoint) bool { return e.DisabledAt == nil }

// ruleTriggerEvents names a Rule version's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(v *ent.RuleVersion) []string { return rules.Triggers(v) }

// ruleVersionOrNull is a version of the Rule, or null when it has none.
func ruleVersionOrNull(c Converter, v *ent.RuleVersion) (siteapi.NilRuleVersion, error) {
	if v == nil {
		return siteapi.NilRuleVersion{Null: true}, nil
	}
	version, err := c.RuleVersionToResource(v)
	if err != nil {
		return siteapi.NilRuleVersion{}, err
	}
	return siteapi.NewNilRuleVersion(version), nil
}

// optNonEmpty is a text that is absent when empty.
func optNonEmpty(s string) siteapi.OptString {
	return convert.OptNonZero[siteapi.OptString](s)
}

// ruleActions maps a Rule's Actions into the typed DTO, an http Action's secrets masked.
func ruleActions(c Converter, actions []ruleaction.Action) []siteapi.RuleAction {
	return lo.Map(actions, func(a ruleaction.Action, _ int) siteapi.RuleAction {
		out := siteapi.RuleAction{Type: c.RuleActionTypeToResource(a.Type), Tag: optNonEmpty(a.Tag)}
		if a.Type == ruleaction.Enroll {
			out.AutomationId = siteapi.NewOptEntityId(siteapi.EntityId(a.AutomationID))
		}
		if a.Type == ruleaction.HTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = optNonEmpty(a.ID)
			out.Body = optNonEmpty(a.Body)
			out.HasSecret = siteapi.NewOptBool(a.Secret != "")
			out.Headers = lo.Map(a.Headers, func(h httpheader.Header, _ int) siteapi.AutomationHttpHeader {
				return siteapi.AutomationHttpHeader{Name: h.Name}
			})
		}
		return out
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO; stored text the module
// cannot decode yields none and is logged, as ruleActions does.
func ruleResults(c Converter, stored string) []siteapi.RuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) siteapi.RuleActionResult {
		return siteapi.RuleActionResult{
			Type:   c.RuleActionTypeToResource(r.Type),
			Status: c.RuleActionStatusToResource(r.Status),
			Detail: optNonEmpty(r.Detail),
		}
	})
}

func optNilOptOutVia(c Converter, v *unsubscribe.Via) siteapi.OptNilOptOutVia {
	return convert.OptVia[siteapi.OptNilOptOutVia](v, c.OptOutViaToResource)
}

func optNilEngagementTrackingProvenance(c Converter, v *engagementtracking.Provenance) siteapi.OptNilEngagementTrackingProvenance {
	return convert.OptVia[siteapi.OptNilEngagementTrackingProvenance](v, c.EngagementTrackingProvenanceToResource)
}

func optNilHoldReason(c Converter, v *holdreason.Reason) siteapi.OptNilHoldReason {
	return convert.OptVia[siteapi.OptNilHoldReason](v, c.HoldReasonToResource)
}

// customFieldConditionTests is what a Condition may ask of a Custom field of the type: the
// Condition's table, listed so a client offers what the server takes.
func customFieldConditionTests(c Converter, t customfield.Type) siteapi.CustomFieldConditionTests {
	return c.ConditionTestsToResource(condition.CustomFieldTests(t))
}

// optAttentionSubject is the object an attention item is about, absent for the Workspace's own.
func optAttentionSubject(c Converter, v *needsattentionreport.Subject) siteapi.OptNilSiteAttentionSubject {
	return convert.OptVia[siteapi.OptNilSiteAttentionSubject](v, c.AttentionSubjectToResource)
}

// optAbuseWarning is the rate behind a deliverability warning, absent on every other item.
func optAbuseWarning(c Converter, v *abusewarning.Warning) siteapi.OptNilAbuseWarning {
	return convert.OptVia[siteapi.OptNilAbuseWarning](v, c.AbuseWarningToResource)
}

// optMessageRevenue is the revenue of a message, present wherever the module read it.
func optMessageRevenue(c Converter, r orders.Revenue) siteapi.OptMessageRevenue {
	return siteapi.NewOptMessageRevenue(c.MessageRevenueToResource(r))
}

func windowHours(d time.Duration) int32 { return narrow.Int32(d / time.Hour) }

func nilFloat64(v *float64) siteapi.NilFloat64 {
	return convert.Nil[siteapi.NilFloat64](v)
}

// revision is a row's revision (ADR 0119) as the contract carries it.
func revision(v int64) siteapi.Revision {
	return siteapi.Revision(v)
}

// ownershipRecordType is the type of a Tracking domain's ownership record (ADR 0173).
func ownershipRecordType(trackingdomains.OwnershipRecord) siteapi.DnsRecordType {
	return siteapi.DnsRecordTypeTXT
}

// contactCsvExportURL is the path the browser follows to download a CSV export of Contacts: the
// contract's SiteContactCsvExports download under the /site prefix (ADR 0138).
func contactCsvExportURL(l contactsexport.Link) string {
	return paths.Site + "/workspaces/" + url.PathEscape(l.Workspace) + "/contact-exports/" + url.PathEscape(l.Token)
}

// envelopeMailFrom is an envelope's custom MAIL FROM, null for any other envelope.
func envelopeMailFrom(c Converter, v *messaging.MailFrom) siteapi.NilSendingDomainMailFrom {
	if v == nil {
		return siteapi.NilSendingDomainMailFrom{Null: true}
	}
	return siteapi.NewNilSendingDomainMailFrom(c.SendingDomainMailFromToResource(*v))
}

// envelopePermission is the permission an unknown envelope needs, null for any other envelope.
func envelopePermission(v string) siteapi.NilString {
	if v == "" {
		return siteapi.NilString{Null: true}
	}
	return siteapi.NewNilString(v)
}

// envelopeCheckedAt is when an envelope was read, null when it never was.
func envelopeCheckedAt(v *time.Time) siteapi.NilTimestamp {
	return convert.Nil[siteapi.NilTimestamp]((*siteapi.Timestamp)(v))
}
