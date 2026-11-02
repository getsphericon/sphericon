// Package resources holds goverter-generated mappers from ent entities to site
// API resources (the ogen *Resource DTOs), plus the custom conversions goverter
// cannot infer on its own. The generated implementation (ConverterImpl) lives in
// converter_gen.go; consumers instantiate it (see internal/api/site).
package resources

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/signupitem"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/abusewatch"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/attention"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/holdreason"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/trackingdomains"

	"github.com/go-faster/jx"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/joindomains"
	"github.com/getsphericon/sphericon/internal/linkclicks"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/setupchecklist"
	"github.com/getsphericon/sphericon/internal/signupitems"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
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
// goverter:extend optNilString
// goverter:extend optNilContactSource
// goverter:extend optNilHoldReason
// goverter:extend omitEmpty
// goverter:extend optNilEmailAddress
// goverter:extend optNilEntityID
// goverter:extend optNilTimeZone
// goverter:extend optNilCountry
// goverter:extend optNilLocale
// goverter:extend optNilImageURL
// goverter:extend optNilDate
// goverter:extend nilCurrencyCode
// goverter:extend nilTimeZone
// goverter:extend attributionWindowDays
// goverter:extend workspaceTimezone
// goverter:extend optNilTimestamp
// goverter:extend nilTimestamp
// goverter:extend optNilInt32
// goverter:extend jsonObject
// goverter:extend editorDocument
// goverter:extend automationSteps
// goverter:extend segmentCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend emailVerified
// goverter:extend transactionalChannel
// goverter:extend transactionalStatus
// goverter:extend transactionalError
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend maxPerDay
// goverter:extend providerLimitSource
// goverter:extend broadcastLimit
// goverter:extend optNilSendCap
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optBool
// goverter:extend optEditorDocument
// goverter:extend optMembershipSource
// goverter:extend optMessageRevenue
// goverter:extend optAttentionSubject
// goverter:extend optAbuseWarning
// goverter:extend optTrackingReason
// goverter:extend timestampOf
// goverter:extend tagNames
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend optConsentDecision
// goverter:extend durationSeconds
type Converter interface {
	// The contact is read with its tags loaded (`WithTags`).
	// goverter:map Edges.Tags Tags
	ContactToResource(source *ent.Contact) siteapi.SiteContactResource
	// goverter:useEmptyOnNil:slice
	ContactsToResources(source []*ent.Contact) []siteapi.SiteContactResource
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) siteapi.SiteSegmentResource
	// goverter:useEmptyOnNil:slice
	SegmentsToResources(source []*ent.Segment) []siteapi.SiteSegmentResource
	TokenToResource(source *ent.ApiToken) siteapi.SiteApiTokenResource
	// goverter:useEmptyOnNil:slice
	TokensToResources(source []*ent.ApiToken) siteapi.SiteTokensListOKApplicationJSON
	// A Connection is an API token read with its Membership and that Membership's User loaded
	// (apitokens.Connections); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	ConnectionToResource(source *ent.ApiToken) siteapi.SiteConnectionResource
	// goverter:useEmptyOnNil:slice
	ConnectionsToResources(source []*ent.ApiToken) siteapi.SiteConnectionsListOKApplicationJSON
	// A Personal token is an API token read with its Membership and that Membership's User loaded
	// (apitokens.PersonalTokens, MintPersonal); the User is who it belongs to.
	// goverter:map Edges.Membership.UserID UserId
	// goverter:map Edges.Membership.Edges.User.Name UserName
	// goverter:map Edges.Membership.Edges.User.Email UserEmail
	PersonalTokenToResource(source *ent.ApiToken) siteapi.SitePersonalTokenResource
	// goverter:useEmptyOnNil:slice
	PersonalTokensToResources(source []*ent.ApiToken) siteapi.SitePersonalTokensListOKApplicationJSON

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
	// The Send cap shown is the one the send path enforces (outbound.Module.SendCapStatus), which
	// the mapper cannot see: the handler attaches it through SendCapStatusToResource.
	// goverter:ignore SendCap
	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	WorkspaceToResource(source *ent.Workspace) siteapi.SiteWorkspaceResource
	// The revenue settings are two columns of the Workspace (ADR 0064).
	RevenueSettingsToResource(source *ent.Workspace) siteapi.SiteRevenueSettingsResource
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
	AnalyticsOverviewToResource(source *analytics.Overview) *siteapi.SiteAnalyticsOverview
	AnalyticsContactsToResource(source analytics.Contacts) siteapi.SiteAnalyticsContacts
	AnalyticsPeriodToResource(source analytics.Period) siteapi.SiteAnalyticsPeriod
	// goverter:map Sent SentCount
	// goverter:map Opened OpenedCount
	// goverter:map Clicked ClickedCount
	AnalyticsEmailToResource(source analytics.Email) siteapi.SiteAnalyticsEmail
	// The Events and Clicks by link reports are lists, which goverter cannot map onto their
	// envelopes: the handler builds those, of one list each.
	// goverter:useEmptyOnNil:slice
	AnalyticsEventActionsToResources(source []analytics.EventAction) []siteapi.SiteAnalyticsEventAction
	// goverter:useEmptyOnNil:slice
	AnalyticsSubscribedPeopleToResource(source *analytics.SubscribedPeople) *siteapi.SiteAnalyticsSubscribedPeople
	// goverter:useEmptyOnNil:slice
	AnalyticsLinksToResources(source []analytics.LinkClicks) []siteapi.SiteAnalyticsLink
	// goverter:useEmptyOnNil:slice
	AnalyticsOpensByHourToResource(source *analytics.OpensByHour) *siteapi.SiteAnalyticsOpensByHour
	// goverter:useEmptyOnNil:slice
	// goverter:map Series Timeseries
	AnalyticsNewPeopleToResource(source *analytics.NewPeople) *siteapi.SiteNewPeopleReport
	AbuseWarningToResource(source abusewatch.Warning) siteapi.SiteAbuseWarning

	// What needs attention maps whole from the module's items; an absent subject, start, hold
	// reason or rate is left out.
	// goverter:useEmptyOnNil:slice
	AttentionItemsToResources(source []attention.Item) []siteapi.SiteAttentionItem
	AttentionSubjectToResource(source attention.Subject) siteapi.SiteAttentionSubject

	EventToResource(source *ent.Event) siteapi.SiteEventResource
	// goverter:useEmptyOnNil:slice
	EventsToResources(source []*ent.Event) []siteapi.SiteEventResource
	LinkClicksToResource(source []linkclicks.Clicks) []siteapi.SiteLinkClicks
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:autoMap Credited
	// goverter:map Currency ReportingCurrency
	MessageRevenueToResource(source orders.Revenue) siteapi.SiteMessageRevenue
	AutomationReportToResource(source automations.Report) siteapi.SiteAutomationReport
	// goverter:autoMap ResultCounts
	// goverter:map AverageDuration AverageDurationSeconds
	AutomationPeriodResultsToResource(source automations.PeriodResults) siteapi.SiteAutomationPeriodResults
	// goverter:autoMap ResultCounts
	AutomationResultsPointToResource(source automations.ResultsPoint) siteapi.SiteAutomationResultsPoint
	// The period is the one the request named: the handler attaches it.
	AutomationResultsToResource(source automations.Results) siteapi.SiteAutomationResults

	// The delivery figures are derived from the Broadcast's recipients and their Outbound
	// messages, which the mapper cannot see: the handler attaches them (BroadcastStatsToResource).
	// goverter:ignore Stats
	// Progress is derived from the recipients, which the mapper cannot see: the
	// handler attaches it.
	// goverter:ignore Progress
	// Warnings are the lint of the content just saved (ADR 0040): the handler attaches them
	// to the response of a save.
	// goverter:ignore Warnings
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	BroadcastToResource(source *ent.Broadcast) siteapi.SiteBroadcastResource

	// A Broadcast's delivery figures; the rates are Stats methods, ratios in [0,1].
	// goverter:map Recipients RecipientsTotal
	// goverter:map Sent SentCount
	// goverter:map Opened OpenedCount
	// goverter:map Clicked ClickedCount
	// goverter:map Unsubscribed UnsubscribedCount
	// goverter:map Failed FailedCount
	// goverter:map Delivered DeliveredCount
	// goverter:map Bounced BouncedCount
	// goverter:map Complained ComplainedCount
	// goverter:map Captured CapturedCount
	BroadcastStatsToResource(source broadcasts.Stats) siteapi.SiteBroadcastStats

	// goverter:map Processed ProcessedCount
	// goverter:map Remaining RemainingCount
	// goverter:map EstimatedCompletion EstimatedCompletionAt
	BroadcastProgressToResource(source broadcasts.Progress) siteapi.SiteBroadcastProgress
	// A Broadcast's progress names only the Send cap or the Provider limit, never the provider's
	// own throttling, which only a Deferral reports.
	// goverter:enum:map CauseSendCap SiteBroadcastLimitSendCap
	// goverter:enum:map CauseProviderLimit SiteBroadcastLimitProviderLimit
	// goverter:enum:map CauseProvider @panic
	BroadcastLimitToResource(source providerlimit.Cause) siteapi.SiteBroadcastLimit

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

	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// The live version and the draft are the edges the automations module loads (ADR 0065).
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	// The revenue is summed from Order Movements, which the mapper cannot see: an Automation the
	// automations module read with its revenue maps through AutomationWithRevenueToResource.
	// goverter:ignore Revenue
	AutomationToResource(source *ent.Automation) siteapi.SiteAutomationResource
	// An Automation with the revenue the automations module read for it (ADR 0064).
	// goverter:autoMap Automation
	// goverter:map Automation.DiscardedAt ArchivedAt
	// goverter:map Automation.Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Automation.Edges.DraftVersion Draft | automationVersionOrNull
	AutomationWithRevenueToResource(source automations.WithRevenue) siteapi.SiteAutomationResource
	// goverter:useEmptyOnNil:slice
	AutomationsWithRevenueToResources(source []automations.WithRevenue) []siteapi.SiteAutomationResource
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map Steps | automationSteps
	// goverter:map IntegrationID IntegrationId
	AutomationVersionToResource(source *ent.AutomationVersion) siteapi.SiteAutomationVersion
	// A step is the module's masked view: what it hides is never mapped.
	// goverter:ignore URL
	// goverter:ignore Secret
	// goverter:ignore ClearSecret
	AutomationStepToResource(source automations.StepView) siteapi.SiteAutomationStep
	AutomationStepsToResources(source []automations.StepView) []siteapi.SiteAutomationStep
	// goverter:ignore Value
	AutomationHttpHeaderToResource(source automations.HeaderName) siteapi.SiteAutomationHttpHeader

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Provider limit from providerlimit.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore ProviderLimit
	// goverter:map DiscardedAt ArchivedAt
	IntegrationToResource(source *ent.Integration) siteapi.SiteIntegrationResource
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) siteapi.SiteSmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) siteapi.SiteSesConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) siteapi.SiteProviderLimitStatus
	// goverter:map Sent24h SentLast24h
	SendCapToResource(source providerlimit.SendCapStatus) siteapi.SiteSendCap
	// The Send cap that applies (outbound.Module.SendCapStatus), null when none does.
	SendCapStatusToResource(source *providerlimit.SendCapStatus) siteapi.OptNilSiteSendCap
	PasskeyToResource(source *ent.Passkey) siteapi.SitePasskey
	// goverter:useEmptyOnNil:slice
	PasskeysToResources(source []*ent.Passkey) []siteapi.SitePasskey
	SuppressionToResource(source *ent.Suppression) siteapi.SiteSuppressionResource
	// goverter:useEmptyOnNil:slice
	SuppressionsToResources(source []*ent.Suppression) []siteapi.SiteSuppressionResource
	// goverter:map Type RuleOperators | customFieldRuleOperators
	CustomFieldToResource(source *ent.CustomField) siteapi.SiteCustomFieldResource
	// goverter:useEmptyOnNil:slice
	CustomFieldsToResources(source []*ent.CustomField) []siteapi.SiteCustomFieldResource
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
	MembershipsToResources(source []*ent.Membership) siteapi.SiteMembershipsListOKApplicationJSON

	// goverter:map . InvitedByEmail | inviterEmail
	InvitationToResource(source *ent.Invitation) siteapi.SiteInvitationResource
	// goverter:useEmptyOnNil:slice
	InvitationsToResources(source []*ent.Invitation) siteapi.SiteInvitationsListOKApplicationJSON

	// The contract names the discarded state "archived" (ADR 0049). The live version and the draft
	// are the edges the rules module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) siteapi.SiteRuleResource
	// goverter:useEmptyOnNil:slice
	RulesToResources(source []*ent.Rule) []siteapi.SiteRuleResource

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) siteapi.SiteRuleVersion

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) siteapi.SiteRuleFiringResource
	// goverter:useEmptyOnNil:slice
	RuleFiringsToResources(source []*ent.RuleFiring) []siteapi.SiteRuleFiringResource

	RuleDryRunToResource(source rules.DryRun) siteapi.SiteRuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	EnrollmentToResource(source *ent.AutomationRun) siteapi.SiteEnrollmentResource
	// goverter:useEmptyOnNil:slice
	EnrollmentsToResources(source []*ent.AutomationRun) []siteapi.SiteEnrollmentResource

	// The deletion window: the date the Workspace's data will be erased.
	// goverter:map PublicID ID
	// goverter:map DiscardedAt EraseAt | eraseAt
	// Whether the caller is its only Owner comes from the Memberships, which the mapper
	// cannot see: the handler attaches it.
	// goverter:ignore OnlyOwner
	DeletedWorkspaceToResource(source *ent.Workspace) siteapi.SiteDeletedWorkspaceResource

	// goverter:map CreatedAt Since
	HoldToResource(source *ent.Hold) siteapi.SiteWorkspaceHold
	// goverter:useEmptyOnNil:slice
	HoldsToResources(source []*ent.Hold) []siteapi.SiteWorkspaceHold

	// goverter:map . Record | joinDomainRecord
	// goverter:map . Verified | joinDomainVerified
	JoinDomainToResource(source *ent.JoinDomain) siteapi.SiteJoinDomainResource
	// goverter:useEmptyOnNil:slice
	JoinDomainsToResources(source []*ent.JoinDomain) []siteapi.SiteJoinDomainResource

	// The DNS records are derived by the sendingdomains module, which the mapper cannot
	// see: the handler attaches them. The private key is never exposed.
	// goverter:ignore DkimRecord
	// goverter:ignore SpfRecord
	// goverter:ignore DmarcRecord
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) siteapi.SiteSendingDomainResource
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) siteapi.SiteDnsRecord

	// The CNAME target is the module's, which the mapper cannot see: the handler attaches it.
	// goverter:ignore Record
	// goverter:map CheckedAt LastCheckedAt
	TrackingDomainToResource(source *ent.TrackingDomain) siteapi.SiteTrackingDomainResource
	// goverter:map . Type | cnameRecordType
	CnameRecordToResource(source trackingdomains.Record) siteapi.SiteCnameRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkit.Kit) siteapi.SiteBrandKitResource

	// goverter:map . Actor
	// goverter:map . Target
	// goverter:map Diff Diff | auditDiff
	AuditEntryToResource(source auditapi.View) siteapi.SiteAuditEntryResource
	// goverter:useEmptyOnNil:slice
	AuditEntriesToResources(source []auditapi.View) []siteapi.SiteAuditEntryResource
	// An entry's actor and target are columns of the entry, nested in the contract.
	// goverter:map ActorKind Kind
	// goverter:map ActorID ID
	// goverter:map ActorName Name
	AuditActorToResource(source auditapi.View) siteapi.SiteAuditActor
	// goverter:map TargetType Type
	// goverter:map TargetID ID
	// goverter:map TargetName Name
	AuditTargetToResource(source auditapi.View) siteapi.SiteAuditTarget
	// goverter:useEmptyOnNil:slice
	AuditFacetsToResource(source auditapi.Facets) siteapi.SiteAuditFacets
	AuditFacetActorToResource(source auditapi.Actor) siteapi.SiteAuditActor

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
	InstanceReleaseToResource(source instance.Release) siteapi.SiteInstanceRelease

	// Pages: a list response is the module's pagination.Page mapped whole, its items through
	// the list methods above. Items the handler maps itself (an extra per item) arrive as a
	// page of resources (pagination.Map), an unpaginated list as pagination.Whole.
	// goverter:useEmptyOnNil:slice
	ContactsListPage(source pagination.Page[*ent.Contact]) *siteapi.SiteContactsListOK
	// goverter:useEmptyOnNil:slice
	SegmentContactsListPage(source pagination.Page[*ent.Contact]) *siteapi.SiteSegmentContactsListOK
	// goverter:useEmptyOnNil:slice
	SegmentsListPage(source pagination.Page[*ent.Segment]) *siteapi.SiteSegmentsListOK
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
	EnrollmentsListPage(source pagination.Page[*ent.AutomationRun]) *siteapi.SiteEnrollmentsListOK
	// goverter:useEmptyOnNil:slice
	RulesListPage(source pagination.Page[*ent.Rule]) *siteapi.SiteRulesListOK
	// goverter:useEmptyOnNil:slice
	RuleFiringsListPage(source pagination.Page[*ent.RuleFiring]) *siteapi.SiteRuleFiringsListOK
	// goverter:useEmptyOnNil:slice
	SuppressionsListPage(source pagination.Page[*ent.Suppression]) *siteapi.SiteSuppressionsListOK
	// goverter:useEmptyOnNil:slice
	AutomationsListPage(source pagination.Page[automations.WithRevenue]) *siteapi.SiteAutomationsListOK
	// goverter:useEmptyOnNil:slice
	EventsListPage(source pagination.Page[*ent.Event]) *siteapi.SiteEventsListOK
	// goverter:useEmptyOnNil:slice
	TemplatesListPage(source pagination.Page[*ent.EmailTemplate]) *siteapi.SiteTemplatesListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	WebhooksListPage(source pagination.Page[siteapi.SiteWebhookEndpointResource]) *siteapi.SiteWebhooksListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	SendingDomainsListPage(source pagination.Page[siteapi.SiteSendingDomainResource]) *siteapi.SiteSendingDomainsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	BroadcastsListPage(source pagination.Page[siteapi.SiteBroadcastResource]) *siteapi.SiteBroadcastsListOK

	SetupChecklistToResource(source setupchecklist.Checklist) siteapi.SiteSetupProgress

	// The Permissions a Role holds, and the one each requested OAuth scope needs.
	// goverter:useEmptyOnNil:slice
	PermissionsToResources(source []accounts.Permission) []siteapi.Permission
	// goverter:useEmptyOnNil:map
	RequiredPermissionsToResource(source map[string]accounts.Permission) siteapi.SiteOAuthAuthorizationRequestRequiredPermissions

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map ReportPeriod7Days SiteAutomationReportPeriod7d
	// goverter:enum:map ReportPeriod14Days SiteAutomationReportPeriod14d
	// goverter:enum:map ReportPeriod30Days SiteAutomationReportPeriod30d
	// goverter:enum:map ReportPeriod90Days SiteAutomationReportPeriod90d
	AutomationReportPeriodToResource(source automations.ReportPeriod) siteapi.SiteAutomationReportPeriod
	// goverter:enum:transform value
	AbuseMetricToResource(source abusewatch.Metric) siteapi.SiteAbuseMetric
	// goverter:enum:transform value
	AttentionKindToResource(source attention.Kind) siteapi.SiteAttentionKind
	// goverter:enum:transform value
	AttentionSeverityToResource(source attention.Severity) siteapi.SiteAttentionSeverity
	// goverter:enum:transform value
	AnalyticsOwnerKindToResource(source linkclicks.OwnerKind) siteapi.SiteAnalyticsOwnerKind
	// goverter:enum:transform value
	AutomationStatusToResource(source automation.Status) siteapi.SiteAutomationStatus
	// goverter:enum:transform value
	AutomationStepTypeToResource(source automations.StepType) siteapi.SiteAutomationStepType
	// goverter:enum:transform value
	BroadcastStatusToResource(source broadcast.Status) siteapi.SiteBroadcastStatus
	// goverter:enum:transform value
	CapturedMessageSurfaceToResource(source capturedmessage.Surface) siteapi.SiteCapturedMessageSurface
	// goverter:enum:transform value
	ContactExportSourceToResource(source contact.Source) siteapi.ContactExportSource
	// goverter:enum:transform value
	HoldReasonToResource(source holdreason.Reason) siteapi.HoldReason
	// goverter:enum:transform value
	CustomFieldTypeToResource(source customfield.Type) siteapi.SiteCustomFieldType
	// What a test send of a Webhook endpoint failed with: a delivery's own error, not the one the
	// endpoint keeps (WebhookErrorKindToResource).
	// goverter:enum:transform value
	DeliveryErrorKindToResource(source webhook.ErrorKind) siteapi.SiteWebhookErrorKind
	// goverter:enum:transform value
	EnrollmentFailureKindToResource(source automationrun.FailureKind) siteapi.SiteEnrollmentFailureKind
	// goverter:enum:transform value
	EnrollmentStatusToResource(source automationrun.Status) siteapi.SiteEnrollmentStatus
	// goverter:enum:transform value
	IntegrationChannelToResource(source integration.Channel) siteapi.SiteIntegrationChannel
	// goverter:enum:transform value
	IntegrationProviderToResource(source integration.Provider) siteapi.SiteIntegrationProvider
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
	PermissionToResource(source accounts.Permission) siteapi.Permission
	// goverter:enum:transform value
	ProviderLimitSourceToResource(source providerlimit.Source) siteapi.SiteProviderLimitSource
	// goverter:enum:transform value
	ProviderLimitWarningToResource(source providerlimit.Warning) siteapi.SiteProviderLimitWarning
	// goverter:enum:transform value
	RuleActionStatusToResource(source rules.Outcome) siteapi.SiteRuleActionStatus
	// goverter:enum:transform value
	RuleActionTypeToResource(source rules.ActionType) siteapi.SiteRuleActionType
	// goverter:enum:transform value
	RuleDryRunOutcomeToResource(source rules.DryRunOutcome) siteapi.SiteRuleDryRunOutcome
	// goverter:enum:transform value
	RuleStatusToResource(source rule.Status) siteapi.SiteRuleStatus
	// goverter:enum:transform value
	SetupStepStatusToResource(source setupchecklist.Status) siteapi.SiteSetupStepStatus
	// goverter:enum:transform value
	SignupItemKindToResource(source signupitem.Kind) siteapi.SignupItemKind
	// goverter:enum:transform value
	SuppressionChannelToResource(source suppression.Channel) siteapi.SiteSuppressionChannel
	// goverter:enum:transform value
	SuppressionReasonToResource(source suppression.Reason) siteapi.SiteSuppressionReason
	// goverter:enum:transform value
	TrackingReasonToResource(source trackingdomain.Reason) siteapi.SiteTrackingReason
	// goverter:enum:transform value
	TrackingStateToResource(source trackingdomain.State) siteapi.SiteTrackingState
	// goverter:enum:transform value
	TrackingDomainRoleToResource(source trackingdomain.Role) siteapi.SiteTrackingDomainRole
	// goverter:enum:transform value
	WebhookDisabledReasonToResource(source webhookendpoint.DisabledReason) siteapi.SiteWebhookDisabledReason
	// goverter:enum:transform value
	WebhookErrorKindToResource(source webhookendpoint.LastErrorKind) siteapi.SiteWebhookErrorKind
}

// ExportMapper adapts the Converter to contactexport.Mapper.
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

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

// maxPerDay narrows a rolling 24-hour limit to the contract's bounded int32.
func maxPerDay(n int) siteapi.MaxPerDay { return siteapi.MaxPerDay(narrow.Int32(n)) }

// nilString renders an optional value: null when there is none.
func nilString(v *string) siteapi.NilString {
	return convert.Nil[siteapi.NilString](v)
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
func providerLimitSource(c Converter, v providerlimit.Source) siteapi.NilSiteProviderLimitSource {
	if v == "" {
		return siteapi.NilSiteProviderLimitSource{Null: true}
	}
	return siteapi.NewNilSiteProviderLimitSource(c.ProviderLimitSourceToResource(v))
}

// optNilSendCap is null exactly when no Send cap applies.
func optNilSendCap(c Converter, v *providerlimit.SendCapStatus) siteapi.OptNilSiteSendCap {
	return convert.NilVia[siteapi.OptNilSiteSendCap](v, c.SendCapToResource)
}

// broadcastLimit is null exactly when nothing limits the Broadcast (its limit is empty).
func broadcastLimit(c Converter, v providerlimit.Cause) siteapi.NilSiteBroadcastLimit {
	if v == "" {
		return siteapi.NilSiteBroadcastLimit{Null: true}
	}
	return siteapi.NewNilSiteBroadcastLimit(c.BroadcastLimitToResource(v))
}

func optNilInt32(v *int) siteapi.OptNilInt32 {
	return convert.Opt[siteapi.OptNilInt32](convert.Int32Ptr(v))
}

// jsonObject renders a stored JSON object (custom fields, event properties): absent when it has
// no keys.
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
func transactionalChannel(outboundmessage.Channel) siteapi.SiteTransactionalEmailChannel {
	return siteapi.SiteTransactionalEmailChannelEmail
}

// transactionalStatus maps an Outbound message's status onto the transactional
// send status enum.
func transactionalStatus(m ent.OutboundMessage) siteapi.SiteTransactionalEmailStatus {
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
func transactionalError(m ent.OutboundMessage) siteapi.OptNilString {
	if m.Status != outboundmessage.StatusFailed || m.Reason == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*m.Reason)
}

func optNilString(v *string) siteapi.OptNilString {
	return convert.Opt[siteapi.OptNilString](v)
}

func optNilEmailAddress(v *string) siteapi.OptNilEmailAddress {
	return convert.Opt[siteapi.OptNilEmailAddress]((*siteapi.EmailAddress)(v))
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

func attributionWindowDays(v int) siteapi.AttributionWindowDays {
	return siteapi.AttributionWindowDays(narrow.Int32(v))
}

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
func automationVersionOrNull(c Converter, v *ent.AutomationVersion) siteapi.NilSiteAutomationVersion {
	if v == nil {
		return siteapi.NilSiteAutomationVersion{Null: true}
	}
	return siteapi.NewNilSiteAutomationVersion(c.AutomationVersionToResource(v))
}

// automationSteps decodes the stored definition (the executor's JSON format) into the typed steps
// DTO, through the automations module's masked view of each step.
func automationSteps(c Converter, definition string) []siteapi.SiteAutomationStep {
	return c.AutomationStepsToResources(automations.Views(definition))
}

func optBool(v *bool) siteapi.OptBool {
	return convert.Opt[siteapi.OptBool](v)
}

func optEditorDocument(doc json.RawMessage) siteapi.OptEditorDocument {
	return convert.OptObject[siteapi.OptEditorDocument](convert.DocObject[siteapi.EditorDocument](doc))
}

// segmentCondition decodes the stored rule group into the typed DTO. A segment with no
// definition (or one that no longer decodes) reads as having none.
func segmentCondition(def *string) siteapi.OptSiteConditionGroup {
	if def == nil || *def == "" {
		return siteapi.OptSiteConditionGroup{}
	}
	var group siteapi.SiteConditionGroup
	if err := condition.Decode(*def, &group); err != nil {
		return siteapi.OptSiteConditionGroup{}
	}
	return siteapi.NewOptSiteConditionGroup(group)
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

func optMembershipSource(c Converter, v *membership.Source) siteapi.OptSiteMembershipSource {
	return convert.OptVia[siteapi.OptSiteMembershipSource](v, c.MembershipSourceToResource)
}

// optConsentDecision is a User's current answer to an optional consent, absent with none; it is
// never `shown`, which only a notice is.
func optConsentDecision(c Converter, v *consentrecord.Decision) siteapi.OptSiteOptionalConsentDecision {
	return convert.OptVia[siteapi.OptSiteOptionalConsentDecision](v, c.OptionalConsentDecisionToResource)
}

func optTrackingReason(c Converter, v *trackingdomain.Reason) siteapi.OptSiteTrackingReason {
	return convert.OptVia[siteapi.OptSiteTrackingReason](v, c.TrackingReasonToResource)
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
	return siteapi.Timestamp(workspacedeletion.EraseDate(*discardedAt))
}

// inviterEmail is the inviting User's address when the inviter edge is loaded.
func inviterEmail(inv ent.Invitation) siteapi.OptNilString {
	if inv.Edges.Inviter == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(inv.Edges.Inviter.Email)
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(c Converter, r ent.AutomationRun) siteapi.OptSiteEnrollmentFailure {
	if r.FailureKind == nil {
		return siteapi.OptSiteEnrollmentFailure{}
	}
	return siteapi.NewOptSiteEnrollmentFailure(siteapi.SiteEnrollmentFailure{
		Kind:    c.EnrollmentFailureKindToResource(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

// joinDomainRecord is the TXT record the Owner publishes to prove the domain.
func joinDomainRecord(d ent.JoinDomain) siteapi.SiteDnsRecord {
	rec := joindomains.RecordOf(&d)
	return siteapi.SiteDnsRecord{Type: siteapi.SiteDnsRecordTypeTXT, Host: rec.Host, Value: rec.Value}
}

func joinDomainVerified(d ent.JoinDomain) bool { return joindomains.Verified(&d) }

func dnsRecordType(sendingdomains.Record) siteapi.SiteDnsRecordType {
	return siteapi.SiteDnsRecordTypeTXT
}

func cnameRecordType(trackingdomains.Record) siteapi.SiteCnameRecordType {
	return siteapi.SiteCnameRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkit.Kit) siteapi.OptNilTimestamp {
	if !k.Saved {
		return siteapi.OptNilTimestamp{}
	}
	return siteapi.NewOptNilTimestamp(siteapi.Timestamp(k.UpdatedAt))
}

func auditDiff(d map[string]jx.Raw) siteapi.OptNilJsonObject {
	return convert.OptObject[siteapi.OptNilJsonObject](siteapi.JsonObject(d))
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(c Converter, e ent.WebhookEndpoint) siteapi.OptSiteWebhookFailure {
	if e.ConsecutiveFailures == nil || e.FailingSince == nil || e.LastFailureAt == nil || e.LastErrorKind == nil {
		return siteapi.OptSiteWebhookFailure{}
	}
	return siteapi.NewOptSiteWebhookFailure(siteapi.SiteWebhookFailure{
		ConsecutiveFailures: narrow.Int32(*e.ConsecutiveFailures),
		Since:               siteapi.Timestamp(*e.FailingSince),
		LastFailureAt:       siteapi.Timestamp(*e.LastFailureAt),
		Kind:                c.WebhookErrorKindToResource(*e.LastErrorKind),
		Status:              optInt32(e.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(c Converter, v *webhookendpoint.DisabledReason) siteapi.OptSiteWebhookDisabledReason {
	return convert.OptVia[siteapi.OptSiteWebhookDisabledReason](v, c.WebhookDisabledReasonToResource)
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) siteapi.OptNilString {
	return convert.OptNonZero[siteapi.OptNilString](s)
}

func smtpConfigKind(integrations.RedactedSMTP) siteapi.SiteSmtpConfigKind {
	return siteapi.SiteSmtpConfigKindSMTP
}

func sesConfigKind(integrations.RedactedSES) siteapi.SiteSesConfigKind {
	return siteapi.SiteSesConfigKindSes
}

// sendingDomainVerified derives the verified flag from the verified and lost moments.
func sendingDomainVerified(d ent.SendingDomain) bool { return dkimstate.Is(&d) }

// webhookEnabled derives the enabled flag: an endpoint is enabled until it has a disabled moment.
func webhookEnabled(e ent.WebhookEndpoint) bool { return e.DisabledAt == nil }

// ruleTriggerEvents names a Rule version's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(v *ent.RuleVersion) []string { return rules.Triggers(v) }

// ruleVersionOrNull is a version of the Rule, or null when it has none.
func ruleVersionOrNull(c Converter, v *ent.RuleVersion) siteapi.NilSiteRuleVersion {
	if v == nil {
		return siteapi.NilSiteRuleVersion{Null: true}
	}
	return siteapi.NewNilSiteRuleVersion(c.RuleVersionToResource(v))
}

// optNonEmpty is a text that is absent when empty.
func optNonEmpty(s string) siteapi.OptString {
	return convert.OptNonZero[siteapi.OptString](s)
}

// ruleActions decodes a Rule's stored Actions into the typed DTO. Stored text the module cannot
// decode (only ever written by the module itself) yields none, and is logged: a converter has
// no way to fail a response, and nobody reading the page can act on it.
func ruleActions(c Converter, stored string) []siteapi.SiteRuleAction {
	actions, err := rules.DecodeActions(stored)
	if err != nil {
		slog.Error("rule: stored actions cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(actions, func(a rules.Action, _ int) siteapi.SiteRuleAction {
		out := siteapi.SiteRuleAction{Type: c.RuleActionTypeToResource(a.Type), Tag: optNonEmpty(a.Tag)}
		if a.Type == rules.ActionEnroll {
			out.AutomationId = siteapi.NewOptEntityId(siteapi.EntityId(a.AutomationID))
		}
		if a.Type == rules.ActionHTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = optNonEmpty(a.ID)
			out.Body = optNonEmpty(a.Body)
			out.HasSecret = siteapi.NewOptBool(a.Secret != "")
			out.Headers = lo.Map(a.Headers, func(h httpcall.Header, _ int) siteapi.SiteAutomationHttpHeader {
				return siteapi.SiteAutomationHttpHeader{Name: h.Name}
			})
		}
		return out
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO; stored text the module
// cannot decode yields none and is logged, as ruleActions does.
func ruleResults(c Converter, stored string) []siteapi.SiteRuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) siteapi.SiteRuleActionResult {
		return siteapi.SiteRuleActionResult{
			Type:   c.RuleActionTypeToResource(r.Type),
			Status: c.RuleActionStatusToResource(r.Status),
			Detail: optNonEmpty(r.Detail),
		}
	})
}

func optNilContactSource(c Converter, v *contact.Source) siteapi.OptNilContactExportSource {
	return convert.OptVia[siteapi.OptNilContactExportSource](v, c.ContactExportSourceToResource)
}

func optNilHoldReason(c Converter, v *holdreason.Reason) siteapi.OptNilHoldReason {
	return convert.OptVia[siteapi.OptNilHoldReason](v, c.HoldReasonToResource)
}

// customFieldRuleOperators is the operators a rule leaf may test on a Custom field of the type: the
// Condition's table, listed so a client offers what the server takes.
func customFieldRuleOperators(t customfield.Type) []siteapi.SiteConditionOperator {
	return lo.Map(condition.RuleOperators(t), func(op string, _ int) siteapi.SiteConditionOperator { return siteapi.SiteConditionOperator(op) })
}

// optAttentionSubject is the object an attention item is about, absent for the Workspace's own.
func optAttentionSubject(c Converter, v *attention.Subject) siteapi.OptNilSiteAttentionSubject {
	return convert.OptVia[siteapi.OptNilSiteAttentionSubject](v, c.AttentionSubjectToResource)
}

// optAbuseWarning is the rate behind a deliverability warning, absent on every other item.
func optAbuseWarning(c Converter, v *abusewatch.Warning) siteapi.OptNilSiteAbuseWarning {
	return convert.OptVia[siteapi.OptNilSiteAbuseWarning](v, c.AbuseWarningToResource)
}

// optMessageRevenue is the revenue of a message, present wherever the module read it.
func optMessageRevenue(c Converter, r orders.Revenue) siteapi.OptSiteMessageRevenue {
	return siteapi.NewOptSiteMessageRevenue(c.MessageRevenueToResource(r))
}
