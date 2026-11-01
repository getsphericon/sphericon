// Package resources holds goverter-generated mappers from ent entities to
// external API resources, plus the custom conversions goverter cannot infer.
// The generated implementation (ConverterImpl) lives in converter_gen.go;
// consumers instantiate it (see internal/api/external).
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
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/orders"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/reportday"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
	"github.com/getsphericon/sphericon/internal/transactional"
	"github.com/getsphericon/sphericon/internal/webhook"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

// automationVersionOrNull is a version of the Automation, or null when it has none.
func automationVersionOrNull(c Converter, v *ent.AutomationVersion) externalapi.NilAutomationVersion {
	if v == nil {
		return externalapi.NilAutomationVersion{Null: true}
	}
	return externalapi.NewNilAutomationVersion(c.AutomationVersionToResource(v))
}

// automationSteps decodes the stored definition (the executor's JSON format) into the typed steps
// DTO, through the automations module's masked view of each step.
func automationSteps(c Converter, definition string) []externalapi.AutomationStep {
	return c.AutomationStepsToResources(automations.Views(definition))
}

func optBool(v *bool) externalapi.OptBool {
	return convert.Opt[externalapi.OptBool](v)
}

func optEditorDocument(doc json.RawMessage) externalapi.OptEditorDocument {
	return convert.OptObject[externalapi.OptEditorDocument](convert.DocObject[externalapi.EditorDocument](doc))
}

// Converter maps ent entities to external API resources. goverter generates the
// implementation; the extends below cover the conversions it can't infer.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optNilString
// goverter:extend optNilSendCap
// goverter:extend optNilContactSource
// goverter:extend optNilEmailAddress
// goverter:extend optNilTimeZone
// goverter:extend optNilTimestamp
// goverter:extend optNilEntityID
// goverter:extend optNilInt32
// goverter:extend jsonObject
// goverter:extend segmentCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend editorDocument
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend maxPerDay
// goverter:extend workspaceTimezone
// goverter:extend omitEmpty
// goverter:extend providerLimitSource
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optBool
// goverter:extend optEditorDocument
// goverter:extend optTrackingReason
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend nilCurrencyCode
// goverter:extend durationSeconds
// goverter:extend eventAction
type Converter interface {
	ContactToResource(source *ent.Contact) externalapi.ContactResource
	// goverter:useEmptyOnNil:slice
	ContactsToResources(source []*ent.Contact) []externalapi.ContactResource
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	BroadcastToResource(source *ent.Broadcast) externalapi.BroadcastResource
	// goverter:useEmptyOnNil:slice
	BroadcastsToResources(source []*ent.Broadcast) []externalapi.BroadcastResource

	// The figures are the report's Stats; the rates are Stats methods.
	// goverter:map Stats.Recipients RecipientsTotal
	// goverter:map Stats.Sent SentCount
	// goverter:map Stats.Skipped SkippedCount
	// goverter:map Stats.Failed FailedCount
	// goverter:map Stats.Opened OpenedCount
	// goverter:map Stats.Clicked ClickedCount
	// goverter:map Stats.Unsubscribed UnsubscribedCount
	// goverter:map Stats.Delivered DeliveredCount
	// goverter:map Stats.Bounced BouncedCount
	// goverter:map Stats.Complained ComplainedCount
	// goverter:map Stats.Captured CapturedCount
	// goverter:map Stats.OpenRate OpenRate
	// goverter:map Stats.ClickRate ClickRate
	BroadcastReportToResource(source broadcasts.Report) externalapi.BroadcastReport
	// The Reporting currency is the Workspace's, null until chosen; the sums are minor units.
	// goverter:autoMap Credited
	// goverter:map Currency ReportingCurrency
	MessageRevenueToResource(source orders.Revenue) externalapi.MessageRevenue
	AutomationReportToResource(source automations.Report) externalapi.AutomationReport
	// goverter:autoMap ResultCounts
	// goverter:map AverageDuration AverageDurationSeconds
	AutomationPeriodResultsToResource(source automations.PeriodResults) externalapi.AutomationPeriodResults
	// goverter:autoMap ResultCounts
	AutomationResultsPointToResource(source automations.ResultsPoint) externalapi.AutomationResultsPoint
	// The period is the one the request named: the handler attaches it.
	AutomationResultsToResource(source automations.Results) externalapi.AutomationResults
	ApiTokenToInfo(source *ent.ApiToken) externalapi.ApiTokenInfo
	// goverter:useEmptyOnNil:slice
	ApiTokensToInfos(source []*ent.ApiToken) []externalapi.ApiTokenInfo
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) externalapi.SegmentResource
	// goverter:useEmptyOnNil:slice
	SegmentsToResources(source []*ent.Segment) []externalapi.SegmentResource
	EmailTemplateToResource(source *ent.EmailTemplate) externalapi.TemplateResource
	// goverter:useEmptyOnNil:slice
	EmailTemplatesToResources(source []*ent.EmailTemplate) []externalapi.TemplateResource

	// A lint finding (ADR 0040) names its element and detail only when the check has one.
	// goverter:map Element Element | optNonEmpty
	// goverter:map Detail Detail | optNonEmpty
	LintFindingToResource(source emailrender.Finding) externalapi.LintFinding
	// goverter:useEmptyOnNil:slice
	LintFindingsToResource(source []emailrender.Finding) []externalapi.LintFinding

	// The editor's document described to an agent (ADR 0040); the example is an opaque object.
	// goverter:map Example Example | schemaExample
	// goverter:useEmptyOnNil:map
	EmailSchemaToResource(source emaildoc.AgentDescription) externalapi.EmailSchemaResult
	// goverter:map Content Content | optNonEmpty
	// goverter:map Group Group | optNonEmpty
	// goverter:useEmptyOnNil:map
	EmailSchemaTypeToResource(source emaildoc.AgentType) externalapi.EmailSchemaType
	// goverter:map Type RuleOperators | customFieldRuleOperators
	CustomFieldToResource(source *ent.CustomField) externalapi.CustomFieldResource
	// goverter:useEmptyOnNil:slice
	CustomFieldsToResources(source []*ent.CustomField) []externalapi.CustomFieldResource
	TagToResource(source *ent.Tag) externalapi.TagResource
	// goverter:useEmptyOnNil:slice
	TagsToResources(source []*ent.Tag) []externalapi.TagResource
	// The contract names the discarded state "archived" (ADR 0049). The live version and the
	// draft are the edges the automations module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	AutomationToResource(source *ent.Automation) externalapi.AutomationResource
	// goverter:useEmptyOnNil:slice
	AutomationsToResources(source []*ent.Automation) []externalapi.AutomationResource
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map Steps | automationSteps
	// goverter:map IntegrationID IntegrationId
	AutomationVersionToResource(source *ent.AutomationVersion) externalapi.AutomationVersion
	// A step is the module's masked view: what it hides is never mapped.
	// goverter:ignore URL
	// goverter:ignore Secret
	// goverter:ignore ClearSecret
	AutomationStepToResource(source automations.StepView) externalapi.AutomationStep
	AutomationStepsToResources(source []automations.StepView) []externalapi.AutomationStep
	// goverter:ignore Value
	AutomationHttpHeaderToResource(source automations.HeaderName) externalapi.AutomationHttpHeader

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Provider limit from providerlimit.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore ProviderLimit
	// goverter:map DiscardedAt ArchivedAt
	IntegrationToResource(source *ent.Integration) externalapi.IntegrationResource

	// A Captured message names its From as the contract does, and the nullable parts as null.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToResource(source *ent.CapturedMessage) externalapi.CapturedMessageResource
	// The list leaves the content out.
	// goverter:map FromEmail From
	// goverter:map IntegrationID IntegrationId
	// goverter:map OutboundMessageID OutboundMessageId | nilEntityID
	// goverter:map FromName FromName | nilString
	// goverter:map SendingSource SendingSource | nilString
	CapturedMessageToSummary(source *ent.CapturedMessage) externalapi.CapturedMessageSummary
	// goverter:useEmptyOnNil:slice
	CapturedMessagesToSummaries(source []*ent.CapturedMessage) []externalapi.CapturedMessageSummary
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) externalapi.SmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) externalapi.SesConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) externalapi.ProviderLimitStatus
	// goverter:map Sent24h SentLast24h
	SendCapToResource(source providerlimit.SendCapStatus) externalapi.SendCap
	// The Send cap that applies (outbound.Module.SendCapStatus), null when none does.
	SendCapStatusToResource(source *providerlimit.SendCapStatus) externalapi.OptNilSendCap

	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	// The Send cap shown is the one the send path enforces (outbound.Module.SendCapStatus), which
	// the mapper cannot see: the handler attaches it through SendCapStatusToResource.
	// goverter:ignore SendCap
	WorkspaceToResource(source *ent.Workspace) externalapi.WorkspaceResource

	// The contract names the discarded state "archived" (ADR 0049). The live version and the draft
	// are the edges the rules module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) externalapi.RuleResource
	// goverter:useEmptyOnNil:slice
	RulesToResources(source []*ent.Rule) []externalapi.RuleResource

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) externalapi.RuleVersion

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) externalapi.RuleFiringResource
	// goverter:useEmptyOnNil:slice
	RuleFiringsToResources(source []*ent.RuleFiring) []externalapi.RuleFiringResource

	RuleDryRunToResource(source rules.DryRun) externalapi.RuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	EnrollmentToResource(source *ent.AutomationRun) externalapi.EnrollmentResource
	// goverter:useEmptyOnNil:slice
	EnrollmentsToResources(source []*ent.AutomationRun) []externalapi.EnrollmentResource

	// The signing secret is deliberately never part of an /api response; the failure state
	// is derived from the failure columns and a nil event filter renders as an empty list.
	// goverter:useEmptyOnNil:slice
	// goverter:map . Failure | webhookFailure
	// goverter:map . Enabled | webhookEnabled
	WebhookToResource(source *ent.WebhookEndpoint) externalapi.WebhookResource

	// The DNS records are derived by the sendingdomains module, which the mapper cannot
	// see: the handler attaches them. The private key is never exposed.
	// goverter:ignore DkimRecord
	// goverter:ignore SpfRecord
	// goverter:ignore DmarcRecord
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) externalapi.SendingDomainResource
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) externalapi.DnsRecord

	// The CNAME target is the module's, which the mapper cannot see: the handler attaches it.
	// goverter:ignore Record
	// goverter:map CheckedAt LastCheckedAt
	TrackingDomainToResource(source *ent.TrackingDomain) externalapi.TrackingDomainResource
	// goverter:map . Type | cnameRecordType
	CnameRecordToResource(source trackingdomains.Record) externalapi.CnameRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkit.Kit) externalapi.BrandKitResource

	// goverter:map . Actor
	// goverter:map . Target
	AuditEntryToResource(source auditapi.View) externalapi.AuditEntryResource
	// goverter:useEmptyOnNil:slice
	AuditEntriesToResources(source []auditapi.View) []externalapi.AuditEntryResource
	// An entry's actor and target are columns of the entry, nested in the contract.
	// goverter:map ActorKind Kind
	// goverter:map ActorID ID
	// goverter:map ActorName Name
	AuditActorToResource(source auditapi.View) externalapi.AuditActor
	// goverter:map TargetType Type
	// goverter:map TargetID ID
	// goverter:map TargetName Name
	AuditTargetToResource(source auditapi.View) externalapi.AuditTarget

	SuppressionToResource(source *ent.Suppression) externalapi.SuppressionResource
	UnsubscribeToResource(source *ent.Unsubscribe) externalapi.UnsubscribeResource

	// The Data export members (ADR 0021): the contract's typed projection of the
	// stored rows, so the ent JSON tags are never the public format.
	ContactToExport(source *ent.Contact) externalapi.ContactExportContact
	TagToExport(source *ent.Tag) externalapi.ContactExportTag
	VisitorToExport(source *ent.Visitor) externalapi.ContactExportVisitor
	EventToExport(source *ent.Event) externalapi.ContactExportEvent
	UnsubscribeToExport(source *ent.Unsubscribe) externalapi.ContactExportUnsubscribe
	SuppressionToExport(source *ent.Suppression) externalapi.ContactExportSuppression
	ConfirmationToExport(source *ent.Confirmation) externalapi.ContactExportConfirmation
	OutboundMessageToExport(source *ent.OutboundMessage) externalapi.ContactExportOutboundMessage
	BroadcastRecipientToExport(source *ent.BroadcastRecipient) externalapi.ContactExportBroadcastRecipient

	// Pages: a list response is the module's pagination.Page mapped whole, its items through
	// the list methods above. Items the handler maps itself (an extra per item) arrive as a
	// page of resources (pagination.Map), an unpaginated list as pagination.Whole.
	// goverter:useEmptyOnNil:slice
	ContactsListPage(source pagination.Page[*ent.Contact]) *externalapi.ContactsListOK
	// goverter:useEmptyOnNil:slice
	SegmentsListPage(source pagination.Page[*ent.Segment]) *externalapi.SegmentsListOK
	// goverter:useEmptyOnNil:slice
	TemplatesListPage(source pagination.Page[*ent.EmailTemplate]) *externalapi.TemplatesListOK
	// goverter:useEmptyOnNil:slice
	BroadcastsListPage(source pagination.Page[*ent.Broadcast]) *externalapi.BroadcastsListOK
	// goverter:useEmptyOnNil:slice
	AutomationsListPage(source pagination.Page[*ent.Automation]) *externalapi.AutomationsListOK
	// goverter:useEmptyOnNil:slice
	EnrollmentsListPage(source pagination.Page[*ent.AutomationRun]) *externalapi.EnrollmentsListOK
	// goverter:useEmptyOnNil:slice
	RulesListPage(source pagination.Page[*ent.Rule]) *externalapi.RulesListOK
	// goverter:useEmptyOnNil:slice
	RuleFiringsListPage(source pagination.Page[*ent.RuleFiring]) *externalapi.RuleFiringsListOK
	// goverter:useEmptyOnNil:slice
	CapturedMessagesListPage(source pagination.Page[*ent.CapturedMessage]) *externalapi.CapturedMessagesListOK
	// goverter:useEmptyOnNil:slice
	TagsListPage(source pagination.Page[*ent.Tag]) *externalapi.TagsListOK
	// goverter:useEmptyOnNil:slice
	TagsListForContactPage(source pagination.Page[*ent.Tag]) *externalapi.TagsListForContactOK
	// goverter:useEmptyOnNil:slice
	CustomFieldsListPage(source pagination.Page[*ent.CustomField]) *externalapi.CustomFieldsListOK
	// goverter:useEmptyOnNil:slice
	EventActionsListPage(source pagination.Page[string]) *externalapi.EventActionsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	WebhooksListPage(source pagination.Page[externalapi.WebhookResource]) *externalapi.WebhooksListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	SendingDomainsListPage(source pagination.Page[externalapi.SendingDomainResource]) *externalapi.SendingDomainsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	IntegrationsListPage(source pagination.Page[externalapi.IntegrationResource]) *externalapi.IntegrationsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	PresetsListPage(source pagination.Page[externalapi.PresetResource]) *externalapi.PresetsListOK
	// goverter:useEmptyOnNil:slice
	// goverter:skipCopySameType
	SendingDomainRatesListPage(source pagination.Page[externalapi.SendingDomainRatesResource]) *externalapi.SendingDomainRatesListOK

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map ReportPeriod7Days AutomationReportPeriod7d
	// goverter:enum:map ReportPeriod14Days AutomationReportPeriod14d
	// goverter:enum:map ReportPeriod30Days AutomationReportPeriod30d
	// goverter:enum:map ReportPeriod90Days AutomationReportPeriod90d
	AutomationReportPeriodToResource(source automations.ReportPeriod) externalapi.AutomationReportPeriod
	// goverter:enum:transform value
	AutomationStatusToResource(source automation.Status) externalapi.AutomationStatus
	// goverter:enum:transform value
	AutomationStepTypeToResource(source automations.StepType) externalapi.AutomationStepType
	// goverter:enum:transform value
	BroadcastStatusToResource(source broadcast.Status) externalapi.BroadcastStatus
	// goverter:enum:transform value
	CapturedMessageSurfaceToResource(source capturedmessage.Surface) externalapi.CapturedMessageSurface
	// goverter:enum:transform value
	ContactExportSourceToResource(source contact.Source) externalapi.ContactExportSource
	// goverter:enum:transform value
	CustomFieldTypeToResource(source customfield.Type) externalapi.CustomFieldType
	// What a test send of a Webhook endpoint failed with: a delivery's own error, not the one the
	// endpoint keeps (WebhookErrorKindToResource).
	// goverter:enum:transform value
	DeliveryErrorKindToResource(source webhook.ErrorKind) externalapi.WebhookErrorKind
	// goverter:enum:transform value
	EnrollmentFailureKindToResource(source automationrun.FailureKind) externalapi.EnrollmentFailureKind
	// goverter:enum:transform value
	EnrollmentStatusToResource(source automationrun.Status) externalapi.EnrollmentStatus
	// goverter:enum:transform value
	IntegrationChannelToResource(source integration.Channel) externalapi.IntegrationChannel
	// goverter:enum:transform value
	IntegrationProviderToResource(source integration.Provider) externalapi.IntegrationProvider
	// goverter:enum:transform value
	LintCodeToResource(source emailrender.Code) externalapi.LintCode
	// goverter:enum:transform value
	ProviderLimitSourceToResource(source providerlimit.Source) externalapi.ProviderLimitSource
	// goverter:enum:transform value
	ProviderLimitWarningToResource(source providerlimit.Warning) externalapi.ProviderLimitWarning
	// goverter:enum:transform value
	RuleActionStatusToResource(source rules.Outcome) externalapi.RuleActionStatus
	// goverter:enum:transform value
	RuleActionTypeToResource(source rules.ActionType) externalapi.RuleActionType
	// goverter:enum:transform value
	RuleDryRunOutcomeToResource(source rules.DryRunOutcome) externalapi.RuleDryRunOutcome
	// goverter:enum:transform value
	RuleStatusToResource(source rule.Status) externalapi.RuleStatus
	// goverter:enum:transform value
	SuppressionReasonToResource(source suppression.Reason) externalapi.SuppressionReason
	// goverter:enum:transform value
	TrackingReasonToResource(source trackingdomain.Reason) externalapi.TrackingReason
	// goverter:enum:transform value
	TransactionalSendStatusToResource(source transactional.Status) externalapi.TransactionalSendStatus
	// goverter:enum:transform value
	TrackingStateToResource(source trackingdomain.State) externalapi.TrackingState
	// goverter:enum:transform value
	TrackingDomainRoleToResource(source trackingdomain.Role) externalapi.TrackingDomainRole
	// goverter:enum:transform value
	WebhookDisabledReasonToResource(source webhookendpoint.DisabledReason) externalapi.WebhookDisabledReason
	// goverter:enum:transform value
	WebhookErrorKindToResource(source webhookendpoint.LastErrorKind) externalapi.WebhookErrorKind
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

// eventAction is one distinct action name of the Workspace's Events.
func eventAction(action string) externalapi.EventActionResource {
	return externalapi.EventActionResource{Action: action}
}

// maxPerDay narrows a rolling 24-hour limit to the contract's bounded int32.
func maxPerDay(n int) externalapi.MaxPerDay { return externalapi.MaxPerDay(narrow.Int32(n)) }

// workspaceTimezone is the Workspace time zone, UTC while an Owner has not set one (ADR 0057).
func workspaceTimezone(v *string) externalapi.TimeZoneName {
	return externalapi.TimeZoneName(reportday.Name(v))
}

// nilCurrencyCode is the Reporting currency, null until an Owner or Admin chooses one (ADR 0064).
func nilCurrencyCode(v *string) externalapi.NilCurrencyCode {
	return convert.Nil[externalapi.NilCurrencyCode]((*externalapi.CurrencyCode)(v))
}

// durationSeconds is a duration in whole seconds, null when there was nothing to measure.
func durationSeconds(d *time.Duration) externalapi.NilInt64 {
	return convert.Nil[externalapi.NilInt64](convert.WholeSeconds(d))
}

// nilString renders an optional value: null when there is none.
func nilString(v *string) externalapi.NilString {
	return convert.Nil[externalapi.NilString](v)
}

// nilEntityID renders an optional reference: null when there is none.
func nilEntityID(v *int64) externalapi.NilEntityId {
	return convert.Nil[externalapi.NilEntityId]((*externalapi.EntityId)(v))
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) externalapi.NilInt32 {
	return convert.Nil[externalapi.NilInt32](convert.Int32Ptr(v))
}

// providerLimitSource is null exactly when the window has no limit (its source is empty).
func providerLimitSource(c Converter, v providerlimit.Source) externalapi.NilProviderLimitSource {
	if v == "" {
		return externalapi.NilProviderLimitSource{Null: true}
	}
	return externalapi.NewNilProviderLimitSource(c.ProviderLimitSourceToResource(v))
}

func optNilInt32(v *int) externalapi.OptNilInt32 {
	return convert.Opt[externalapi.OptNilInt32](convert.Int32Ptr(v))
}

// jsonObject renders a stored JSON object (custom fields, event properties): absent when it has
// no keys.
func jsonObject(m map[string]any) externalapi.OptNilJsonObject {
	return convert.OptObject[externalapi.OptNilJsonObject](convert.RawObject[externalapi.JsonObject](m))
}

// editorDocument is a stored editor document (a Template's, a Broadcast's) as the opaque object of
// the contract; a message written as HTML has none.
func editorDocument(doc json.RawMessage) externalapi.OptNilEditorDocument {
	return convert.OptObject[externalapi.OptNilEditorDocument](convert.DocObject[externalapi.EditorDocument](doc))
}

func timestamp(t time.Time) externalapi.Timestamp {
	return externalapi.Timestamp(t)
}

func optNilString(v *string) externalapi.OptNilString {
	return convert.Opt[externalapi.OptNilString](v)
}

func optNilEmailAddress(v *string) externalapi.OptNilEmailAddress {
	return convert.Opt[externalapi.OptNilEmailAddress]((*externalapi.EmailAddress)(v))
}

func optNilEntityID(v *int64) externalapi.OptNilEntityId {
	return convert.Opt[externalapi.OptNilEntityId]((*externalapi.EntityId)(v))
}

func optNilTimeZone(v *string) externalapi.OptNilTimeZoneName {
	return convert.Opt[externalapi.OptNilTimeZoneName]((*externalapi.TimeZoneName)(v))
}

func optNilTimestamp(v *time.Time) externalapi.OptNilTimestamp {
	return convert.Opt[externalapi.OptNilTimestamp]((*externalapi.Timestamp)(v))
}

// segmentCondition decodes the stored rule group into the typed DTO. A segment with no
// definition (or one that no longer decodes) reads as having none.
func segmentCondition(def *string) externalapi.OptConditionGroup {
	if def == nil || *def == "" {
		return externalapi.OptConditionGroup{}
	}
	var group externalapi.ConditionGroup
	if err := condition.Decode(*def, &group); err != nil {
		return externalapi.OptConditionGroup{}
	}
	return externalapi.NewOptConditionGroup(group)
}

func optTimestamp(v *time.Time) externalapi.OptTimestamp {
	return convert.Opt[externalapi.OptTimestamp]((*externalapi.Timestamp)(v))
}

func optInt32(v *int) externalapi.OptInt32 {
	return convert.Opt[externalapi.OptInt32](convert.Int32Ptr(v))
}

func optString(v *string) externalapi.OptString {
	return convert.Opt[externalapi.OptString](v)
}

func optTrackingReason(c Converter, v *trackingdomain.Reason) externalapi.OptTrackingReason {
	return convert.OptVia[externalapi.OptTrackingReason](v, c.TrackingReasonToResource)
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(c Converter, r ent.AutomationRun) externalapi.OptEnrollmentFailure {
	if r.FailureKind == nil {
		return externalapi.OptEnrollmentFailure{}
	}
	return externalapi.NewOptEnrollmentFailure(externalapi.EnrollmentFailure{
		Kind:    c.EnrollmentFailureKindToResource(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

func dnsRecordType(sendingdomains.Record) externalapi.DnsRecordType {
	return externalapi.DnsRecordTypeTXT
}

func cnameRecordType(trackingdomains.Record) externalapi.CnameRecordType {
	return externalapi.CnameRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkit.Kit) externalapi.OptNilTimestamp {
	if !k.Saved {
		return externalapi.OptNilTimestamp{}
	}
	return externalapi.NewOptNilTimestamp(externalapi.Timestamp(k.UpdatedAt))
}

func auditDiff(d map[string]jx.Raw) externalapi.OptNilJsonObject {
	return convert.OptObject[externalapi.OptNilJsonObject](externalapi.JsonObject(d))
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(c Converter, e ent.WebhookEndpoint) externalapi.OptWebhookFailure {
	if e.ConsecutiveFailures == nil || e.FailingSince == nil || e.LastFailureAt == nil || e.LastErrorKind == nil {
		return externalapi.OptWebhookFailure{}
	}
	return externalapi.NewOptWebhookFailure(externalapi.WebhookFailure{
		ConsecutiveFailures: narrow.Int32(*e.ConsecutiveFailures),
		Since:               externalapi.Timestamp(*e.FailingSince),
		LastFailureAt:       externalapi.Timestamp(*e.LastFailureAt),
		Kind:                c.WebhookErrorKindToResource(*e.LastErrorKind),
		Status:              optInt32(e.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(c Converter, v *webhookendpoint.DisabledReason) externalapi.OptWebhookDisabledReason {
	return convert.OptVia[externalapi.OptWebhookDisabledReason](v, c.WebhookDisabledReasonToResource)
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) externalapi.OptNilString {
	return convert.OptNonZero[externalapi.OptNilString](s)
}

func smtpConfigKind(integrations.RedactedSMTP) externalapi.SmtpConfigKind {
	return externalapi.SmtpConfigKindSMTP
}

func sesConfigKind(integrations.RedactedSES) externalapi.SesConfigKind {
	return externalapi.SesConfigKindSes
}

// sendingDomainVerified derives the verified flag from the verified and lost moments.
func sendingDomainVerified(d ent.SendingDomain) bool { return dkimstate.Is(&d) }

// webhookEnabled derives the enabled flag: an endpoint is enabled until it has a disabled moment.
func webhookEnabled(e ent.WebhookEndpoint) bool { return e.DisabledAt == nil }

// ruleTriggerEvents names a Rule version's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(v *ent.RuleVersion) []string { return rules.Triggers(v) }

// ruleVersionOrNull is a version of the Rule, or null when it has none.
func ruleVersionOrNull(c Converter, v *ent.RuleVersion) externalapi.NilRuleVersion {
	if v == nil {
		return externalapi.NilRuleVersion{Null: true}
	}
	return externalapi.NewNilRuleVersion(c.RuleVersionToResource(v))
}

// optNonEmpty is a text that is absent when empty.
func optNonEmpty(s string) externalapi.OptString {
	return convert.OptNonZero[externalapi.OptString](s)
}

// ruleActions decodes a Rule's stored Actions into the typed DTO. Stored text the module cannot
// decode (only ever written by the module itself) yields none, and is logged: a converter has
// no way to fail a response, and no caller can act on it.
func ruleActions(c Converter, stored string) []externalapi.RuleAction {
	actions, err := rules.DecodeActions(stored)
	if err != nil {
		slog.Error("rule: stored actions cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(actions, func(a rules.Action, _ int) externalapi.RuleAction {
		out := externalapi.RuleAction{Type: c.RuleActionTypeToResource(a.Type), Tag: optNonEmpty(a.Tag)}
		if a.Type == rules.ActionEnroll {
			out.AutomationId = externalapi.NewOptEntityId(externalapi.EntityId(a.AutomationID))
		}
		if a.Type == rules.ActionHTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = optNonEmpty(a.ID)
			out.Body = optNonEmpty(a.Body)
			out.HasSecret = externalapi.NewOptBool(a.Secret != "")
			out.Headers = lo.Map(a.Headers, func(h httpcall.Header, _ int) externalapi.AutomationHttpHeader {
				return externalapi.AutomationHttpHeader{Name: h.Name}
			})
		}
		return out
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO; stored text the module
// cannot decode yields none and is logged, as ruleActions does.
func ruleResults(c Converter, stored string) []externalapi.RuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) externalapi.RuleActionResult {
		return externalapi.RuleActionResult{
			Type:   c.RuleActionTypeToResource(r.Type),
			Status: c.RuleActionStatusToResource(r.Status),
			Detail: optNonEmpty(r.Detail),
		}
	})
}

func optNilContactSource(c Converter, v *contact.Source) externalapi.OptNilContactExportSource {
	return convert.OptVia[externalapi.OptNilContactExportSource](v, c.ContactExportSourceToResource)
}

// schemaExample is the agent schema's example document as the contract's opaque object.
func schemaExample(doc json.RawMessage) externalapi.EditorDocument {
	return convert.DocObject[externalapi.EditorDocument](doc)
}

// customFieldRuleOperators is the operators a rule leaf may test on a Custom field of the type: the
// Condition's table, listed so a client offers what the server takes.
func customFieldRuleOperators(t customfield.Type) []externalapi.ConditionOperator {
	return lo.Map(condition.RuleOperators(t), func(op string, _ int) externalapi.ConditionOperator { return externalapi.ConditionOperator(op) })
}

// optNilSendCap is null exactly when no Send cap applies.
func optNilSendCap(c Converter, v *providerlimit.SendCapStatus) externalapi.OptNilSendCap {
	return convert.NilVia[externalapi.OptNilSendCap](v, c.SendCapToResource)
}
