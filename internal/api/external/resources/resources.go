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
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
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
	"github.com/getsphericon/sphericon/internal/providerlimit"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
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

// automationSteps decodes the steps of an Automation version into the typed steps DTO.
// Malformed steps yield no steps rather than an error.
func automationSteps(v *ent.AutomationVersion) []externalapi.AutomationStep {
	stored, err := automations.Decode(v.Steps)
	if err != nil {
		return nil
	}
	steps := make([]externalapi.AutomationStep, 0, len(stored))
	for i := range stored {
		s := &stored[i]
		step := externalapi.AutomationStep{Type: externalapi.AutomationStepType(s.Type)}
		switch s.Type {
		case automations.StepWait:
			step.Seconds = externalapi.NewOptInt32(narrow.Int32(s.Seconds))
		case automations.StepApplyTag, automations.StepRemoveTag:
			step.Tag = externalapi.NewOptString(s.Tag)
		case automations.StepHTTP:
			// Masked: the URL, header values and signing secret never leave the server.
			step.ID = externalapi.NewOptString(s.ID)
			step.Body = externalapi.NewOptString(s.Body)
			step.HasSecret = externalapi.NewOptBool(s.Secret != "")
			step.Headers = lo.Map(s.Headers, func(h automations.Header, _ int) externalapi.AutomationHttpHeader {
				return externalapi.AutomationHttpHeader{Name: h.Name}
			})
		default:
			step.Subject = externalapi.NewOptString(s.Subject)
			step.Body = externalapi.NewOptString(s.Body)
			if doc := convert.DocObject[externalapi.AutomationStepDoc](s.Doc); doc != nil {
				step.Doc = externalapi.NewOptAutomationStepDoc(doc)
				step.EditorVersion = externalapi.NewOptString(s.EditorVersion)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// Converter maps ent entities to external API resources. goverter generates the
// implementation; the extends below cover the conversions it can't infer.
//
// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:enum no
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optNilString
// goverter:extend optNilContactSource
// goverter:extend optNilEmailAddress
// goverter:extend optNilTimeZone
// goverter:extend optNilTimestamp
// goverter:extend optNilEntityID
// goverter:extend contactCustomFields
// goverter:extend optNilInt32
// goverter:extend exportJSON
// goverter:extend segmentCondition
// goverter:extend ruleActions
// goverter:extend ruleResults
// goverter:extend templateDoc
// goverter:extend broadcastDoc
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend omitEmpty
// goverter:extend providerLimitWarning
// goverter:extend providerLimitSource
// goverter:extend optTimestamp
// goverter:extend optInt32
// goverter:extend optString
// goverter:extend optTrackingReason
// goverter:extend auditDiff
// goverter:extend optWebhookDisabledReason
// goverter:extend nilCurrencyCode
// goverter:extend durationSeconds
type Converter interface {
	ContactToResource(source *ent.Contact) externalapi.ContactResource
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	BroadcastToResource(source *ent.Broadcast) externalapi.BroadcastResource

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
	// goverter:ignore Period
	AutomationResultsToResource(source automations.Results) externalapi.AutomationResults
	ApiTokenToInfo(source *ent.ApiToken) externalapi.ApiTokenInfo
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) externalapi.SegmentResource
	EmailTemplateToResource(source *ent.EmailTemplate) externalapi.TemplateResource

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
	TagToResource(source *ent.Tag) externalapi.TagResource
	// The contract names the discarded state "archived" (ADR 0049). The live version and the
	// draft are the edges the automations module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | automationVersionOrNull
	// goverter:map Edges.DraftVersion Draft | automationVersionOrNull
	AutomationToResource(source *ent.Automation) externalapi.AutomationResource
	// The stored definition string (the executor's JSON format) is decoded into the typed steps
	// DTO at this boundary. The contract names the version "number".
	// goverter:map Version Number | count
	// goverter:map . Steps | automationSteps
	// goverter:map IntegrationID IntegrationId
	AutomationVersionToResource(source *ent.AutomationVersion) externalapi.AutomationVersion

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
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) externalapi.SmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) externalapi.SesConfig
	// goverter:map Sent24h SentLast24h
	ProviderLimitToStatus(source providerlimit.Status) externalapi.ProviderLimitStatus

	// The contract names the discarded state "archived" (ADR 0049). The live version and the draft
	// are the edges the rules module loads (ADR 0065).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map Edges.LiveVersion Live | ruleVersionOrNull
	// goverter:map Edges.DraftVersion Draft | ruleVersionOrNull
	RuleToResource(source *ent.Rule) externalapi.RuleResource

	// The contract numbers a version; its Trigger events are the rows of its `triggers` edge,
	// loaded with it, and the stored actions JSON is decoded into the typed Actions.
	// goverter:map Version Number
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleVersionToResource(source *ent.RuleVersion) externalapi.RuleVersion

	// The store names the Event's source id; the contract says whose it is.
	// goverter:map SourceID EventSourceId
	RuleFiringToResource(source *ent.RuleFiring) externalapi.RuleFiringResource

	RuleDryRunToResource(source rules.DryRun) externalapi.RuleDryRunResource

	// goverter:map . Failure | enrollmentFailure
	EnrollmentToResource(source *ent.AutomationRun) externalapi.EnrollmentResource

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
	// goverter:ignore TrackingRecord
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . Verified | sendingDomainVerified
	SendingDomainToResource(source *ent.SendingDomain) externalapi.SendingDomainResource
	// goverter:map . Type | dnsRecordType
	DnsRecordToResource(source sendingdomains.Record) externalapi.DnsRecord
	// goverter:map . Type | trackingRecordType
	// goverter:map CheckedAt LastCheckedAt
	TrackingRecordToResource(source sendingdomains.TrackingRecord) externalapi.TrackingRecord

	// A Workspace that has not saved a Brand kit has no row to convert, so the mapper
	// takes the module's Kit.
	// goverter:map . UpdatedAt | brandKitUpdatedAt
	BrandKitToResource(source brandkit.Kit) externalapi.BrandKitResource

	// goverter:map . Actor
	// goverter:map . Target
	AuditEntryToResource(source auditapi.View) externalapi.AuditEntryResource
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

// providerLimitWarning casts: providerlimit's warnings are the contract's enum values.
func providerLimitWarning(w providerlimit.Warning) externalapi.ProviderLimitWarning {
	return externalapi.ProviderLimitWarning(w)
}

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

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
func providerLimitSource(v providerlimit.Source) externalapi.NilProviderLimitSource {
	if v == "" {
		return externalapi.NilProviderLimitSource{Null: true}
	}
	return externalapi.NewNilProviderLimitSource(externalapi.ProviderLimitSource(v))
}

func optNilInt32(v *int) externalapi.OptNilInt32 {
	return convert.Opt[externalapi.OptNilInt32](convert.Int32Ptr(v))
}

// exportJSON renders a stored JSON object (custom fields, event properties).
func exportJSON(m map[string]any) externalapi.OptNilContactExportJson {
	return convert.OptObject[externalapi.OptNilContactExportJson](convert.RawObject[externalapi.ContactExportJson](m))
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

func contactCustomFields(m map[string]any) externalapi.OptNilContactResourceCustomFields {
	return convert.OptObject[externalapi.OptNilContactResourceCustomFields](convert.RawObject[externalapi.ContactResourceCustomFields](m))
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

// templateDoc is a Template's stored editor document as the opaque object of the contract; a
// Template written as HTML has none.
func templateDoc(doc json.RawMessage) externalapi.OptNilTemplateResourceDoc {
	return convert.OptObject[externalapi.OptNilTemplateResourceDoc](convert.DocObject[externalapi.TemplateResourceDoc](doc))
}

// broadcastDoc is a Broadcast's stored editor document as the opaque object of the contract; a
// Broadcast written as HTML has none.
func broadcastDoc(doc json.RawMessage) externalapi.OptNilBroadcastResourceDoc {
	return convert.OptObject[externalapi.OptNilBroadcastResourceDoc](convert.DocObject[externalapi.BroadcastResourceDoc](doc))
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

func optTrackingReason(v *sendingdomain.TrackingReason) externalapi.OptTrackingReason {
	return convert.Opt[externalapi.OptTrackingReason]((*externalapi.TrackingReason)(v))
}

// enrollmentFailure is the enrollment's failure detail; absent unless it failed.
func enrollmentFailure(r ent.AutomationRun) externalapi.OptEnrollmentFailure {
	if r.FailureKind == nil {
		return externalapi.OptEnrollmentFailure{}
	}
	return externalapi.NewOptEnrollmentFailure(externalapi.EnrollmentFailure{
		Kind:    externalapi.EnrollmentFailureKind(*r.FailureKind),
		Status:  optInt32(r.FailureStatus),
		Excerpt: optString(r.FailureExcerpt),
	})
}

func dnsRecordType(sendingdomains.Record) externalapi.DnsRecordType {
	return externalapi.DnsRecordTypeTXT
}

func trackingRecordType(sendingdomains.TrackingRecord) externalapi.TrackingRecordType {
	return externalapi.TrackingRecordTypeCNAME
}

// brandKitUpdatedAt is set only once the Brand kit has been saved.
func brandKitUpdatedAt(k brandkit.Kit) externalapi.OptNilTimestamp {
	if !k.Saved {
		return externalapi.OptNilTimestamp{}
	}
	return externalapi.NewOptNilTimestamp(externalapi.Timestamp(k.UpdatedAt))
}

func auditDiff(d map[string]jx.Raw) externalapi.OptNilAuditEntryResourceDiff {
	return convert.OptObject[externalapi.OptNilAuditEntryResourceDiff](externalapi.AuditEntryResourceDiff(d))
}

// webhookFailure is the endpoint's failure state; it is absent while the endpoint is healthy.
func webhookFailure(e ent.WebhookEndpoint) externalapi.OptWebhookFailure {
	if e.ConsecutiveFailures == nil || e.FailingSince == nil || e.LastFailureAt == nil || e.LastErrorKind == nil {
		return externalapi.OptWebhookFailure{}
	}
	return externalapi.NewOptWebhookFailure(externalapi.WebhookFailure{
		ConsecutiveFailures: narrow.Int32(*e.ConsecutiveFailures),
		Since:               externalapi.Timestamp(*e.FailingSince),
		LastFailureAt:       externalapi.Timestamp(*e.LastFailureAt),
		Kind:                externalapi.WebhookErrorKind(*e.LastErrorKind),
		Status:              optInt32(e.LastErrorStatus),
	})
}

// optWebhookDisabledReason is why the endpoint turned itself off; absent otherwise.
func optWebhookDisabledReason(v *webhookendpoint.DisabledReason) externalapi.OptWebhookDisabledReason {
	return convert.Opt[externalapi.OptWebhookDisabledReason]((*externalapi.WebhookDisabledReason)(v))
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
func ruleActions(stored string) []externalapi.RuleAction {
	actions, err := rules.DecodeActions(stored)
	if err != nil {
		slog.Error("rule: stored actions cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(actions, func(a rules.Action, _ int) externalapi.RuleAction {
		out := externalapi.RuleAction{Type: externalapi.RuleActionType(a.Type), Tag: optNonEmpty(a.Tag)}
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
func ruleResults(stored string) []externalapi.RuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
		slog.Error("rule: stored results cannot be decoded", "error", err)
		return nil
	}
	return lo.Map(results, func(r rules.Result, _ int) externalapi.RuleActionResult {
		return externalapi.RuleActionResult{
			Type:   externalapi.RuleActionType(r.Type),
			Status: externalapi.RuleActionStatus(r.Status),
			Detail: optNonEmpty(r.Detail),
		}
	})
}

func optNilContactSource(v *contact.Source) externalapi.OptNilContactExportSource {
	return convert.Opt[externalapi.OptNilContactExportSource]((*externalapi.ContactExportSource)(v))
}

// schemaExample is the agent schema's example document as the contract's opaque object.
func schemaExample(doc json.RawMessage) externalapi.EmailSchemaResultExample {
	return convert.DocObject[externalapi.EmailSchemaResultExample](doc)
}

// customFieldRuleOperators is the operators a rule leaf may test on a Custom field of the type: the
// Condition's table, listed so a client offers what the server takes.
func customFieldRuleOperators(t customfield.Type) []externalapi.ConditionOperator {
	return lo.Map(condition.RuleOperators(t), func(op string, _ int) externalapi.ConditionOperator { return externalapi.ConditionOperator(op) })
}
