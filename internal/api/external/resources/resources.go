// Package resources holds goverter-generated mappers from ent entities to
// external API resources, plus the custom conversions goverter cannot infer.
// The generated implementation (ConverterImpl) lives in converter_gen.go;
// consumers instantiate it (see internal/api/external).
package resources

import (
	"encoding/json"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/sendlimit"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

// automationSteps decodes the steps of the Automation's latest version into the typed steps
// DTO. Malformed or missing steps yield no steps rather than an error.
func automationSteps(a *ent.Automation) []externalapi.AutomationStep {
	stored, err := automations.CurrentSteps(a)
	if err != nil {
		return nil
	}
	steps := make([]externalapi.AutomationStep, 0, len(stored))
	for i := range stored {
		s := &stored[i]
		step := externalapi.AutomationStep{Type: externalapi.AutomationStepType(s.Type)}
		switch s.Type {
		case automations.StepWait:
			step.Seconds = externalapi.NewOptInt32(int32(s.Seconds))
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

// AutomationSteps converts the typed steps DTO to the automations module's steps.
func AutomationSteps(steps []externalapi.AutomationStep) []automations.Step {
	out := make([]automations.Step, 0, len(steps))
	for i := range steps {
		s := &steps[i]
		out = append(out, automations.Step{
			Type:    string(s.Type),
			Subject: s.Subject.Or(""),
			Body:    s.Body.Or(""),
			Doc:     convert.DocJSON(s.Doc.Value, s.Doc.Set),
			Seconds: int(s.Seconds.Or(0)),
			Tag:     s.Tag.Or(""),

			ID:  s.ID.Or(""),
			URL: s.URL.Or(""),
			Headers: lo.Map(s.Headers, func(h externalapi.AutomationHttpHeader, _ int) automations.Header {
				return automations.Header{Name: h.Name, Value: h.Value.Or("")}
			}),
			Secret:      s.Secret.Or(""),
			ClearSecret: s.ClearSecret.Or(false),
		})
	}
	return out
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
// goverter:extend sendLimitWarning
// goverter:extend sendLimitSource
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
	ApiTokenToInfo(source *ent.ApiToken) externalapi.ApiTokenInfo
	// The contract names the discarded state \"archived\" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	SegmentToResource(source *ent.Segment) externalapi.SegmentResource
	EmailTemplateToResource(source *ent.EmailTemplate) externalapi.TemplateResource
	CustomFieldToResource(source *ent.CustomField) externalapi.CustomFieldResource
	TagToResource(source *ent.Tag) externalapi.TagResource
	// The contract names the discarded state "archived" (ADR 0049).
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . Steps | automationSteps
	AutomationToResource(source *ent.Automation) externalapi.AutomationResource

	// The config is a oneOf the generator cannot express, and the params, hook URL and
	// retiring secrets come from the integrations module (decryption, instance URL), and
	// the Send rate limit from sendlimit.Statuses: the handler attaches them.
	// goverter:ignore Config
	// goverter:ignore Params
	// goverter:ignore SecretsRetiring
	// goverter:ignore HookUrl
	// goverter:ignore SendLimit
	// goverter:map DiscardedAt ArchivedAt
	IntegrationToResource(source *ent.Integration) externalapi.IntegrationResource
	// The redacted provider configs: the handler only wraps the one that is set in the
	// contract's oneOf, which goverter cannot express. Empty optional fields are omitted.
	// goverter:map . Kind | smtpConfigKind
	SmtpConfigToResource(source integrations.RedactedSMTP) externalapi.SmtpConfig
	// goverter:map . Kind | sesConfigKind
	SesConfigToResource(source integrations.RedactedSES) externalapi.SesConfig
	// goverter:map Sent24h SentLast24h
	SendLimitToStatus(source sendlimit.Status) externalapi.SendLimitStatus

	// The contract names the discarded state "archived" (ADR 0049). The Trigger events are
	// the rows of the Rule's `triggers` edge, loaded with the Rule; the stored actions JSON
	// is decoded into the typed Actions at this boundary.
	// goverter:map DiscardedAt ArchivedAt
	// goverter:map . TriggerEvents | ruleTriggerEvents
	RuleToResource(source *ent.Rule) externalapi.RuleResource

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

// sendLimitWarning casts: sendlimit's warnings are the contract's enum values.
func sendLimitWarning(w sendlimit.Warning) externalapi.SendLimitWarning {
	return externalapi.SendLimitWarning(w)
}

// count narrows a message count to the contract's int32.
func count(n int) int32 { return int32(n) }

// nilString renders an optional value: null when there is none.
func nilString(v *string) externalapi.NilString {
	if v == nil {
		return externalapi.NilString{Null: true}
	}
	return externalapi.NewNilString(*v)
}

// nilInt32 renders a limit: null when there is none.
func nilInt32(v *int) externalapi.NilInt32 {
	if v == nil {
		return externalapi.NilInt32{Null: true}
	}
	return externalapi.NewNilInt32(int32(*v))
}

// sendLimitSource is null exactly when the window has no limit (its source is empty).
func sendLimitSource(v sendlimit.Source) externalapi.NilSendLimitSource {
	if v == "" {
		return externalapi.NilSendLimitSource{Null: true}
	}
	return externalapi.NewNilSendLimitSource(externalapi.SendLimitSource(v))
}

func optNilInt32(v *int) externalapi.OptNilInt32 {
	if v == nil {
		return externalapi.OptNilInt32{}
	}
	return externalapi.NewOptNilInt32(int32(*v))
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
	return externalapi.NewOptInt32(int32(*v))
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
		ConsecutiveFailures: int32(*e.ConsecutiveFailures),
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

// ruleTriggerEvents names a Rule's Trigger events: the rows of its `triggers` edge.
func ruleTriggerEvents(r *ent.Rule) []string { return rules.Triggers(r) }

// ruleOptString is a text that is absent when empty.
func ruleOptString(s string) externalapi.OptString {
	if s == "" {
		return externalapi.OptString{}
	}
	return externalapi.NewOptString(s)
}

// ruleActions decodes a Rule's stored Actions into the typed DTO. Malformed or missing actions
// yield none rather than an error.
func ruleActions(stored string) []externalapi.RuleAction {
	actions, err := rules.DecodeActions(stored)
	if err != nil {
		return nil
	}
	return lo.Map(actions, func(a rules.Action, _ int) externalapi.RuleAction {
		out := externalapi.RuleAction{Type: externalapi.RuleActionType(a.Type), Tag: ruleOptString(a.Tag)}
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

// RuleActions converts the typed Actions DTO to the Rule module's Actions.
func RuleActions(actions []externalapi.RuleAction) []rules.Action {
	return lo.Map(actions, func(a externalapi.RuleAction, _ int) rules.Action {
		return rules.Action{
			Type: string(a.Type), Tag: a.Tag.Or(""),
			ID: a.ID.Or(""), URL: a.URL.Or(""), Body: a.Body.Or(""),
			Headers: lo.Map(a.Headers, func(h externalapi.AutomationHttpHeader, _ int) httpcall.Header {
				return httpcall.Header{Name: h.Name, Value: h.Value.Or("")}
			}),
			Secret: a.Secret.Or(""), ClearSecret: a.ClearSecret.Or(false),
		}
	})
}

// ruleResults decodes a Firing's stored results into the typed DTO.
func ruleResults(stored string) []externalapi.RuleActionResult {
	results, err := rules.DecodeResults(stored)
	if err != nil {
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
