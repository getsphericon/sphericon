// Package resources holds goverter-generated mappers from ent entities to
// external API resources, plus the custom conversions goverter cannot infer.
// The generated implementation (ConverterImpl) lives in converter_gen.go;
// consumers instantiate it (see internal/api/external).
package resources

import (
	"encoding/json"
	"time"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/segments"
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
	for _, s := range stored {
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
	for _, s := range steps {
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
// goverter:extend segmentDefinition
// goverter:extend templateDoc
// goverter:extend broadcastDoc
// goverter:extend nilInt32
// goverter:extend nilString
// goverter:extend count
// goverter:extend sendLimitWarning
// goverter:extend sendLimitSource
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
	IntegrationToResource(source *ent.Integration) externalapi.IntegrationResource
	// goverter:map Sent24h SentLast24h
	SendLimitToStatus(source sendlimit.Status) externalapi.SendLimitStatus

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

// segmentDefinition decodes the stored rule group into the typed DTO. A segment with no
// definition (or one that no longer decodes) reads as having none.
func segmentDefinition(def *string) externalapi.OptSegmentRuleGroup {
	if def == nil || *def == "" {
		return externalapi.OptSegmentRuleGroup{}
	}
	var group externalapi.SegmentRuleGroup
	if err := segments.Decode(*def, &group); err != nil {
		return externalapi.OptSegmentRuleGroup{}
	}
	return externalapi.NewOptSegmentRuleGroup(group)
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
