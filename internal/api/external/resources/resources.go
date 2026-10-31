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
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/narrow"
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
// goverter:extend openRate
// goverter:extend clickRate
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
	// goverter:map Stats OpenRate | openRate
	// goverter:map Stats ClickRate | clickRate
	BroadcastReportToResource(source broadcasts.Report) externalapi.BroadcastReport
	ApiTokenToInfo(source *ent.ApiToken) externalapi.ApiTokenInfo
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) externalapi.SegmentResource
	EmailTemplateToResource(source *ent.EmailTemplate) externalapi.TemplateResource
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
	// goverter:map EventTypes EventTypes | eventTypes
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

	// goverter:map . Actor | auditActor
	// goverter:map . Target | auditTarget
	AuditEntryToResource(source auditapi.View) externalapi.AuditEntryResource

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

// openRate and clickRate are the report's rates, which Stats computes.
func openRate(st broadcasts.Stats) float32  { return st.OpenRate() }
func clickRate(st broadcasts.Stats) float32 { return st.ClickRate() }

// count narrows a message count to the contract's int32.
func count(n int) int32 { return narrow.Int32(n) }

// nilString renders an optional value: null when there is none.
func nilString(v *string) externalapi.NilString {
	if v == nil {
		return externalapi.NilString{Null: true}
	}
	return externalapi.NewNilString(*v)
}

// nilEntityID renders an optional reference: null when there is none.
func nilEntityID(v *int64) externalapi.NilEntityId {
	if v == nil {
		return externalapi.NilEntityId{Null: true}
	}
	return externalapi.NewNilEntityId(externalapi.EntityId(*v))
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) externalapi.NilInt32 {
	if v == nil {
		return externalapi.NilInt32{Null: true}
	}
	return externalapi.NewNilInt32(narrow.Int32(*v))
}

// providerLimitSource is null exactly when the window has no limit (its source is empty).
func providerLimitSource(v providerlimit.Source) externalapi.NilProviderLimitSource {
	if v == "" {
		return externalapi.NilProviderLimitSource{Null: true}
	}
	return externalapi.NewNilProviderLimitSource(externalapi.ProviderLimitSource(v))
}

func optNilInt32(v *int) externalapi.OptNilInt32 {
	if v == nil {
		return externalapi.OptNilInt32{}
	}
	return externalapi.NewOptNilInt32(narrow.Int32(*v))
}

// exportJSON renders a stored JSON object (custom fields, event properties).
func exportJSON(m map[string]any) externalapi.OptNilContactExportJson {
	if len(m) == 0 {
		return externalapi.OptNilContactExportJson{}
	}
	out := make(externalapi.ContactExportJson, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out[k] = jx.Raw(b)
	}
	return externalapi.NewOptNilContactExportJson(out)
}

func timestamp(t time.Time) externalapi.Timestamp {
	return externalapi.Timestamp(t)
}

func optNilString(v *string) externalapi.OptNilString {
	if v == nil {
		return externalapi.OptNilString{}
	}
	return externalapi.NewOptNilString(*v)
}

func optNilEmailAddress(v *string) externalapi.OptNilEmailAddress {
	if v == nil {
		return externalapi.OptNilEmailAddress{}
	}
	return externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(*v))
}

func optNilEntityID(v *int64) externalapi.OptNilEntityId {
	if v == nil {
		return externalapi.OptNilEntityId{}
	}
	return externalapi.NewOptNilEntityId(externalapi.EntityId(*v))
}

func optNilTimeZone(v *string) externalapi.OptNilTimeZoneName {
	if v == nil {
		return externalapi.OptNilTimeZoneName{}
	}
	return externalapi.NewOptNilTimeZoneName(externalapi.TimeZoneName(*v))
}

func optNilTimestamp(v *time.Time) externalapi.OptNilTimestamp {
	if v == nil {
		return externalapi.OptNilTimestamp{}
	}
	return externalapi.NewOptNilTimestamp(externalapi.Timestamp(*v))
}

func contactCustomFields(m map[string]any) externalapi.OptNilContactResourceCustomFields {
	if len(m) == 0 {
		return externalapi.OptNilContactResourceCustomFields{}
	}
	fields := make(externalapi.ContactResourceCustomFields, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		fields[k] = jx.Raw(b)
	}
	return externalapi.NewOptNilContactResourceCustomFields(fields)
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
	if fields := convert.DocObject[externalapi.TemplateResourceDoc](doc); fields != nil {
		return externalapi.NewOptNilTemplateResourceDoc(fields)
	}
	return externalapi.OptNilTemplateResourceDoc{}
}

// broadcastDoc is a Broadcast's stored editor document as the opaque object of the contract; a
// Broadcast written as HTML has none.
func broadcastDoc(doc json.RawMessage) externalapi.OptNilBroadcastResourceDoc {
	if fields := convert.DocObject[externalapi.BroadcastResourceDoc](doc); fields != nil {
		return externalapi.NewOptNilBroadcastResourceDoc(fields)
	}
	return externalapi.OptNilBroadcastResourceDoc{}
}

func optTimestamp(v *time.Time) externalapi.OptTimestamp {
	if v == nil {
		return externalapi.OptTimestamp{}
	}
	return externalapi.NewOptTimestamp(externalapi.Timestamp(*v))
}

func optInt32(v *int) externalapi.OptInt32 {
	if v == nil {
		return externalapi.OptInt32{}
	}
	return externalapi.NewOptInt32(narrow.Int32(*v))
}

func optString(v *string) externalapi.OptString {
	if v == nil {
		return externalapi.OptString{}
	}
	return externalapi.NewOptString(*v)
}

func optTrackingReason(v *sendingdomain.TrackingReason) externalapi.OptTrackingReason {
	if v == nil {
		return externalapi.OptTrackingReason{}
	}
	return externalapi.NewOptTrackingReason(externalapi.TrackingReason(*v))
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

func auditActor(v auditapi.View) externalapi.AuditActor {
	return externalapi.AuditActor{Kind: externalapi.AuditActorKind(v.ActorKind), ID: optNilString(v.ActorID), Name: optNilString(v.ActorName)}
}

func auditTarget(v auditapi.View) externalapi.AuditTarget {
	return externalapi.AuditTarget{Type: v.TargetType, ID: optNilString(v.TargetID), Name: optNilString(v.TargetName)}
}

func auditDiff(d map[string]jx.Raw) externalapi.OptNilAuditEntryResourceDiff {
	if d == nil {
		return externalapi.OptNilAuditEntryResourceDiff{}
	}
	return externalapi.NewOptNilAuditEntryResourceDiff(d)
}

// eventTypes shows an endpoint with no event filter as an empty list, never null.
func eventTypes(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
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
	if v == nil {
		return externalapi.OptWebhookDisabledReason{}
	}
	return externalapi.NewOptWebhookDisabledReason(externalapi.WebhookDisabledReason(*v))
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) externalapi.OptNilString {
	if s == "" {
		return externalapi.OptNilString{}
	}
	return externalapi.NewOptNilString(s)
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

// ruleOptString is a text that is absent when empty.
func ruleOptString(s string) externalapi.OptString {
	if s == "" {
		return externalapi.OptString{}
	}
	return externalapi.NewOptString(s)
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
		out := externalapi.RuleAction{Type: externalapi.RuleActionType(a.Type), Tag: ruleOptString(a.Tag)}
		if a.Type == rules.ActionEnroll {
			out.AutomationId = externalapi.NewOptEntityId(externalapi.EntityId(a.AutomationID))
		}
		if a.Type == rules.ActionHTTP {
			// Masked: the URL, header values and signing secret never leave the server.
			out.ID = ruleOptString(a.ID)
			out.Body = ruleOptString(a.Body)
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
			Detail: ruleOptString(r.Detail),
		}
	})
}

func optNilContactSource(v *contact.Source) externalapi.OptNilContactExportSource {
	if v == nil {
		return externalapi.OptNilContactExportSource{}
	}
	return externalapi.NewOptNilContactExportSource(externalapi.ContactExportSource(*v))
}
