// Package resources holds goverter-generated mappers from ent entities to site
// API resources (the ogen *Resource DTOs), plus the custom conversions goverter
// cannot infer on its own. The generated implementation (ConverterImpl) lives in
// converter_gen.go; consumers instantiate it (see internal/api/site).
package resources

import (
	"encoding/json"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

// Converter maps ent entities to site API resources. goverter generates the
// implementation; named string/time casts are inferred, the extends below cover
// the conversions it can't (ids, optionals, the event property bag).
//
// goverter:converter
// goverter:output:file ./converter_gen.go
// goverter:matchIgnoreCase
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend timestamp
// goverter:extend optNilString
// goverter:extend optNilEmailAddress
// goverter:extend optNilEntityID
// goverter:extend optNilTimeZone
// goverter:extend optNilTimestamp
// goverter:extend contactCustomFields
// goverter:extend optNilInt32
// goverter:extend exportJSON
// goverter:extend eventProperties
// goverter:extend templateDoc
// goverter:extend broadcastDoc
// goverter:extend automationSteps
// goverter:extend segmentDefinition
// goverter:extend emailVerified
// goverter:extend transactionalChannel
// goverter:extend transactionalStatus
// goverter:extend transactionalError
type Converter interface {
	ContactToResource(source *ent.Contact) siteapi.SiteContactResource
	SegmentToResource(source *ent.Segment) siteapi.SiteSegmentResource
	TokenToResource(source *ent.ApiToken) siteapi.SiteApiTokenResource

	// Verification is derived: a non-nil email_verified_at means verified.
	// goverter:map EmailVerifiedAt EmailVerified | emailVerified
	UserToResource(source *ent.User) *siteapi.SiteUserResource
	// The grace end and the role belong to the authenticated User's Membership,
	// which the mapper cannot see: the handler attaches them.
	// goverter:ignore SecondFactorGraceEndsAt
	// goverter:ignore Role
	// goverter:ignore AccountLinkAvailable
	// The active holds are read through the Workspace's scope, which the mapper cannot
	// see: the handler attaches them.
	// goverter:ignore Holds
	// The support contact is instance configuration, attached by the handler.
	// goverter:ignore SupportEmail
	// The MCP origin is instance configuration, attached by the handler.
	// goverter:ignore McpUrl
	// The numeric key stays internal: the contract's id is the Workspace id.
	// goverter:map PublicID ID
	WorkspaceToResource(source *ent.Workspace) siteapi.SiteWorkspaceResource
	EventToResource(source *ent.Event) siteapi.SiteEventResource

	// The delivery figures are derived from the Broadcast's recipients and their Outbound
	// messages, which the mapper cannot see: the handler attaches them (BroadcastStats).
	// goverter:ignore Stats
	// Progress is derived from the recipients, which the mapper cannot see: the
	// handler attaches it.
	// goverter:ignore Progress
	// Warnings are the lint of the content just saved (ADR 0040): the handler attaches them
	// to the response of a save.
	// goverter:ignore Warnings
	BroadcastToResource(source *ent.Broadcast) siteapi.SiteBroadcastResource

	// goverter:ignore Warnings
	EmailTemplateToResource(source *ent.EmailTemplate) siteapi.SiteEmailTemplateResource

	// The stored definition string (the executor's JSON format) is decoded into
	// the typed steps DTO at this boundary.
	// goverter:map Definition Steps | automationSteps
	AutomationToResource(source *ent.Automation) siteapi.SiteAutomationResource
	SuppressionToResource(source *ent.Suppression) siteapi.SiteSuppressionResource
	CustomFieldToResource(source *ent.CustomField) siteapi.SiteCustomFieldResource
	TagToResource(source *ent.Tag) siteapi.SiteTagResource

	// The transactional send history is the Outbound messages of kind transactional
	// (ADR 0015). A message skipped by Send-eligibility is shown as "suppressed" (the
	// only reason a transactional send is skipped); the reason column is the error
	// text only for a failed send.
	// goverter:map . Status | transactionalStatus
	// goverter:map . Error | transactionalError
	// goverter:map TemplateID TemplateId
	TransactionalMessageToResource(source *ent.OutboundMessage) siteapi.SiteTransactionalEmailResource

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

func optNilInt32(v *int) siteapi.OptNilInt32 {
	if v == nil {
		return siteapi.OptNilInt32{}
	}
	return siteapi.NewOptNilInt32(int32(*v))
}

// exportJSON renders a stored JSON object (custom fields, event properties).
func exportJSON(m map[string]any) siteapi.OptNilContactExportJson {
	if len(m) == 0 {
		return siteapi.OptNilContactExportJson{}
	}
	out := make(siteapi.ContactExportJson, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		out[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilContactExportJson(out)
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
	if v == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*v)
}

func optNilEmailAddress(v *string) siteapi.OptNilEmailAddress {
	if v == nil {
		return siteapi.OptNilEmailAddress{}
	}
	return siteapi.NewOptNilEmailAddress(siteapi.EmailAddress(*v))
}

func optNilEntityID(v *int64) siteapi.OptNilEntityId {
	if v == nil {
		return siteapi.OptNilEntityId{}
	}
	return siteapi.NewOptNilEntityId(siteapi.EntityId(*v))
}

// BroadcastStats nests a Broadcast's delivery figures in the Stats DTO and computes the
// derived engagement rates. Rates are ratios in [0,1]; a zero denominator yields 0.
func BroadcastStats(st broadcasts.Stats) siteapi.SiteBroadcastStats {
	return siteapi.SiteBroadcastStats{
		RecipientsTotal:   int32(st.Recipients),
		SentCount:         int32(st.Sent),
		OpenedCount:       int32(st.Opened),
		ClickedCount:      int32(st.Clicked),
		UnsubscribedCount: int32(st.Unsubscribed),
		FailedCount:       int32(st.Failed),
		DeliveryRate:      broadcasts.Ratio(st.Sent, st.Recipients),
		OpenRate:          broadcasts.Ratio(st.Opened, st.Sent),
		ClickRate:         broadcasts.Ratio(st.Clicked, st.Sent),
		ClickToOpenRate:   broadcasts.Ratio(st.Clicked, st.Opened),
		UnsubscribeRate:   broadcasts.Ratio(st.Unsubscribed, st.Sent),
		FailureRate:       broadcasts.Ratio(st.Failed, st.Recipients),
	}
}

func optNilTimeZone(v *string) siteapi.OptNilTimeZoneName {
	if v == nil {
		return siteapi.OptNilTimeZoneName{}
	}
	return siteapi.NewOptNilTimeZoneName(siteapi.TimeZoneName(*v))
}

func optNilTimestamp(v *time.Time) siteapi.OptNilTimestamp {
	if v == nil {
		return siteapi.OptNilTimestamp{}
	}
	return siteapi.NewOptNilTimestamp(siteapi.Timestamp(*v))
}

func contactCustomFields(m map[string]any) siteapi.OptNilSiteContactResourceCustomFields {
	if len(m) == 0 {
		return siteapi.OptNilSiteContactResourceCustomFields{}
	}
	fields := make(siteapi.SiteContactResourceCustomFields, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		fields[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilSiteContactResourceCustomFields(fields)
}

// templateDoc is a Template's stored editor document as the opaque object of the contract; a
// Template written as HTML has none.
func templateDoc(doc json.RawMessage) siteapi.OptNilSiteEmailTemplateResourceDoc {
	if len(doc) == 0 {
		return siteapi.OptNilSiteEmailTemplateResourceDoc{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return siteapi.OptNilSiteEmailTemplateResourceDoc{}
	}
	out := make(siteapi.SiteEmailTemplateResourceDoc, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return siteapi.NewOptNilSiteEmailTemplateResourceDoc(out)
}

// broadcastDoc is a Broadcast's stored editor document as the opaque object of the contract; a
// Broadcast written as HTML has none.
func broadcastDoc(doc json.RawMessage) siteapi.OptNilSiteBroadcastResourceDoc {
	if len(doc) == 0 {
		return siteapi.OptNilSiteBroadcastResourceDoc{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return siteapi.OptNilSiteBroadcastResourceDoc{}
	}
	out := make(siteapi.SiteBroadcastResourceDoc, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return siteapi.NewOptNilSiteBroadcastResourceDoc(out)
}

func eventProperties(m map[string]any) siteapi.OptNilSiteEventResourceProperties {
	if len(m) == 0 {
		return siteapi.OptNilSiteEventResourceProperties{}
	}
	props := make(siteapi.SiteEventResourceProperties, len(m))
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		props[k] = jx.Raw(b)
	}
	return siteapi.NewOptNilSiteEventResourceProperties(props)
}

// automationSteps decodes the stored definition string into the typed steps DTO.
// A malformed/empty definition yields no steps rather than an error.
func automationSteps(def string) []siteapi.SiteAutomationStep {
	stored, err := automations.Decode(def)
	if err != nil {
		return nil
	}
	steps := make([]siteapi.SiteAutomationStep, 0, len(stored))
	for _, s := range stored {
		step := siteapi.SiteAutomationStep{Type: siteapi.SiteAutomationStepType(s.Type)}
		switch s.Type {
		case automations.StepWait:
			step.Seconds = siteapi.NewOptInt32(int32(s.Seconds))
		case automations.StepApplyTag, automations.StepRemoveTag:
			step.Tag = siteapi.NewOptString(s.Tag)
		case automations.StepHTTP:
			// Masked: the URL, header values and signing secret never leave the server.
			step.ID = siteapi.NewOptString(s.ID)
			step.Body = siteapi.NewOptString(s.Body)
			step.HasSecret = siteapi.NewOptBool(s.Secret != "")
			step.Headers = lo.Map(s.Headers, func(h automations.Header, _ int) siteapi.SiteAutomationHttpHeader {
				return siteapi.SiteAutomationHttpHeader{Name: h.Name}
			})
		default:
			step.Subject = siteapi.NewOptString(s.Subject)
			step.Body = siteapi.NewOptString(s.Body)
			if doc := stepDoc(s.Doc); doc != nil {
				step.Doc = siteapi.NewOptSiteAutomationStepDoc(doc)
				step.EditorVersion = siteapi.NewOptString(s.EditorVersion)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

// AutomationSteps converts the typed steps DTO to the automations module's steps.
func AutomationSteps(steps []siteapi.SiteAutomationStep) []automations.Step {
	out := make([]automations.Step, 0, len(steps))
	for _, s := range steps {
		out = append(out, automations.Step{
			Type:    string(s.Type),
			Subject: s.Subject.Or(""),
			Body:    s.Body.Or(""),
			Doc:     docJSON(s.Doc),
			Seconds: int(s.Seconds.Or(0)),
			Tag:     s.Tag.Or(""),

			ID:  s.ID.Or(""),
			URL: s.URL.Or(""),
			Headers: lo.Map(s.Headers, func(h siteapi.SiteAutomationHttpHeader, _ int) automations.Header {
				return automations.Header{Name: h.Name, Value: h.Value.Or("")}
			}),
			Secret:      s.Secret.Or(""),
			ClearSecret: s.ClearSecret.Or(false),
		})
	}
	return out
}

// stepDoc is an email step's stored editor document as the opaque object of the contract; nil
// for a step written as HTML.
func stepDoc(doc json.RawMessage) siteapi.SiteAutomationStepDoc {
	if len(doc) == 0 {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(doc, &fields); err != nil {
		return nil
	}
	out := make(siteapi.SiteAutomationStepDoc, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return out
}

// docJSON is a request's opaque document object as the JSON the module renders; nil when the
// step names none. An empty object is a document (an invalid one), not an absent one.
func docJSON(o siteapi.OptSiteAutomationStepDoc) json.RawMessage {
	if !o.Set {
		return nil
	}
	raw := make(map[string]json.RawMessage, len(o.Value))
	for k, v := range o.Value {
		raw[k] = json.RawMessage(v)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return b
}

// segmentDefinition decodes the stored rule group into the typed DTO. A segment with no
// definition (or one that no longer decodes) reads as having none.
func segmentDefinition(def *string) siteapi.OptSiteSegmentRuleGroup {
	if def == nil || *def == "" {
		return siteapi.OptSiteSegmentRuleGroup{}
	}
	var group siteapi.SiteSegmentRuleGroup
	if err := segments.Decode(*def, &group); err != nil {
		return siteapi.OptSiteSegmentRuleGroup{}
	}
	return siteapi.NewOptSiteSegmentRuleGroup(group)
}
