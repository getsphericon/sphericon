package resources

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// Inputs maps request bodies onto the modules' inputs: the input direction of the contract, so a
// handler only hands the module what the request says. An update becomes the module's patch
// (internal/patch): an absent key keeps, an explicit null clears, a value sets. The extends below
// bind the generated option types of this surface to the shared conversions of internal/convert.
//
// goverter:converter
// goverter:annotate:unmapped
// goverter:name InputsImpl
// goverter:output:file ./inputs_gen.go
// goverter:matchIgnoreCase
// goverter:enum:unknown @panic
// goverter:useZeroValueOnPointerInconsistency
// goverter:extend stringPatch
// goverter:extend nilStringPatch
// goverter:extend nilEmailPatch
// goverter:extend nilTimeZonePatch
// goverter:extend nilEntityIDPatch
// goverter:extend boolPatch
// goverter:extend maxPerSecondPatch
// goverter:extend maxPerDayPatch
// goverter:extend listPatch
// goverter:extend stringIn
// goverter:extend stringPtrIn
// goverter:extend nilStringPtrIn
// goverter:extend nilEmailPtrIn
// goverter:extend nilTimeZonePtrIn
// goverter:extend nilEntityIDPtrIn
// goverter:extend boolIn
// goverter:extend boolPtrIn
// goverter:extend int32In
// goverter:extend entityIDIn
// goverter:extend maxPerSecondPtrIn
// goverter:extend maxPerDayPtrIn
// goverter:extend integrationConfigIn
// goverter:extend integrationConfigPatch
// goverter:extend listPtrIn
// goverter:extend nilStringIn
// goverter:extend timestampIn
// goverter:extend timestampPtrIn
// goverter:extend automationStepsPtrIn
// goverter:extend ruleActionsPtrIn
type Inputs interface {
	// goverter:map Doc | createTemplateDoc
	// goverter:ignore Prepared
	TemplateCreate(source *externalapi.CreateTemplateInput) templates.CreateInput
	// goverter:map Doc | updateTemplateDoc
	TemplateUpdate(source *externalapi.UpdateTemplateInput) templates.UpdateInput

	// goverter:map Doc | createBroadcastDoc
	BroadcastCreate(source *externalapi.CreateBroadcastInput) broadcasts.CreateInput
	// goverter:map Doc | updateBroadcastDoc
	BroadcastUpdate(source *externalapi.UpdateBroadcastInput) broadcasts.UpdateInput

	WebhookCreate(source *externalapi.CreateWebhookInput) webhooks.CreateInput
	WebhookUpdate(source *externalapi.UpdateWebhookInput) webhooks.UpdateInput

	// goverter:map DkimSelector Selector
	SendingDomainCreate(source *externalapi.CreateSendingDomainInput) sendingdomains.CreateInput
	// goverter:map DkimSelector Selector
	SendingDomainUpdate(source *externalapi.UpdateSendingDomainInput) sendingdomains.UpdateInput

	// /api tags a Contact through its own operations, so its bodies carry no tags.
	// goverter:map . Attributes
	// goverter:ignore Tags
	ContactCreate(source *externalapi.CreateContactInput) contacts.CreateInput
	// goverter:map CustomFields | createContactFields
	ContactAttributes(source externalapi.CreateContactInput) contacts.Attributes
	// goverter:map CustomFields | updateContactFields
	// goverter:ignore Tags
	ContactUpdate(source *externalapi.UpdateContactInput) contacts.UpdateInput
	// goverter:map CustomFields | upsertContactFields
	ContactUpsert(source externalapi.UpsertContactInput) contacts.Attributes
	ContactUpserts(source []externalapi.UpsertContactInput) []contacts.Attributes

	// goverter:map Params | createIntegrationParams
	IntegrationCreate(source *externalapi.CreateIntegrationInput) (integrations.CreateInput, error)
	// goverter:map Params | updateIntegrationParams
	IntegrationUpdate(source *externalapi.UpdateIntegrationInput) (integrations.UpdateInput, error)

	// goverter:map Condition | createSegmentCondition
	SegmentCreate(source *externalapi.CreateSegmentInput) (segments.CreateInput, error)
	// goverter:map Condition | segmentConditionPatch
	SegmentUpdate(source *externalapi.UpdateSegmentInput) (segments.UpdateInput, error)

	// The editor version is the save's own: the module renders the document and stamps it.
	// goverter:map Doc | stepDocIn
	// goverter:ignore EditorVersion
	AutomationStep(source externalapi.AutomationStep) automations.Step
	AutomationSteps(source []externalapi.AutomationStep) []automations.Step
	AutomationCreate(source *externalapi.CreateAutomationInput) automations.CreateInput
	AutomationUpdate(source *externalapi.UpdateAutomationInput) automations.UpdateInput

	RuleActions(source []externalapi.RuleAction) []rules.Action
	// goverter:map Condition | ruleConditionIn
	RuleCreate(source *externalapi.CreateRuleInput) (rules.CreateInput, error)
	// goverter:map Condition | ruleConditionIn
	RuleUpdate(source *externalapi.UpdateRuleInput) (rules.UpdateInput, error)

	ApiTokenCreate(source *externalapi.CreateApiTokenInput) apitokens.Input

	// Backfill is the batch's flag, not an Event's: it is passed along and holds for each Event.
	// goverter:context backfill
	// goverter:map Properties | eventPropertiesIn
	// goverter:map Backfill | backfillIn
	Event(source externalapi.EventInput, backfill bool) (eventlog.Input, error)
	// goverter:context backfill
	Events(source []externalapi.EventInput, backfill bool) ([]eventlog.Input, error)

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// goverter:enum:transform value
	RuleActionType(source externalapi.RuleActionType) rules.ActionType
}

// goverter:context backfill
func backfillIn(backfill bool) bool { return backfill }

func stringPatch(o externalapi.OptString) patch.Field[string]       { return convert.Patch(o) }
func nilStringPatch(o externalapi.OptNilString) patch.Field[string] { return convert.Patch(o) }
func nilEmailPatch(o externalapi.OptNilEmailAddress) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilTimeZonePatch(o externalapi.OptNilTimeZoneName) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilEntityIDPatch(o externalapi.OptNilEntityId) patch.Field[int64]    { return convert.Int64Patch(o) }
func boolPatch(o externalapi.OptBool) patch.Field[bool]                   { return convert.Patch(o) }
func maxPerSecondPatch(o externalapi.OptNilMaxPerSecond) patch.Field[int] { return convert.IntPatch(o) }
func maxPerDayPatch(o externalapi.OptNilMaxPerDay) patch.Field[int]       { return convert.IntPatch(o) }
func listPatch(list []string) patch.Field[[]string]                       { return convert.ListPatch(list) }
func stringIn(o externalapi.OptString) string                             { return o.Or("") }
func stringPtrIn(o externalapi.OptString) *string                         { return convert.Ptr(o) }
func nilStringPtrIn(o externalapi.OptNilString) *string                   { return convert.Ptr(o) }
func nilEmailPtrIn(o externalapi.OptNilEmailAddress) *string              { return convert.StringPtr(o) }
func nilTimeZonePtrIn(o externalapi.OptNilTimeZoneName) *string           { return convert.StringPtr(o) }
func nilEntityIDPtrIn(o externalapi.OptNilEntityId) *int64                { return convert.Int64Ptr(o) }
func boolIn(o externalapi.OptBool) bool                                   { return o.Or(false) }
func boolPtrIn(o externalapi.OptBool) *bool                               { return convert.Ptr(o) }
func int32In(o externalapi.OptInt32) int                                  { return int(o.Or(0)) }
func entityIDIn(o externalapi.OptEntityId) int64                          { return int64(o.Or(0)) }
func maxPerSecondPtrIn(o externalapi.OptNilMaxPerSecond) *int             { return intPtr(o) }
func maxPerDayPtrIn(o externalapi.OptNilMaxPerDay) *int                   { return intPtr(o) }
func nilStringIn(o externalapi.OptNilString) string                       { return o.Or("") }

// listPtrIn is a list an update replaces: an omitted list (nil) keeps the stored one.
func listPtrIn(list []string) *[]string {
	if list == nil {
		return nil
	}
	return &list
}

// timestampIn is a moment the request may leave out: the zero time when absent.
func timestampIn(o externalapi.OptNilTimestamp) time.Time {
	v, _ := o.Get()
	return time.Time(v)
}

func timestampPtrIn(o externalapi.OptNilTimestamp) *time.Time {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	t := time.Time(v)
	return &t
}

// automationStepsPtrIn is the steps an update replaces: omitted (nil) keeps the stored ones.
func automationStepsPtrIn(c Inputs, steps []externalapi.AutomationStep) *[]automations.Step {
	if steps == nil {
		return nil
	}
	out := c.AutomationSteps(steps)
	return &out
}

// ruleActionsPtrIn is the Actions an update replaces: omitted (nil) keeps the stored ones.
func ruleActionsPtrIn(c Inputs, actions []externalapi.RuleAction) *[]rules.Action {
	if actions == nil {
		return nil
	}
	out := c.RuleActions(actions)
	return &out
}

func ruleConditionIn(o externalapi.OptConditionGroup) (*string, error) {
	return condition.EncodeOptional(o)
}

// eventPropertiesIn decodes each property keeping numbers as the text that was sent: an amount of
// an order Event is checked for its decimal places (ADR 0064), which a float64 would blur.
func eventPropertiesIn(o externalapi.OptNilEventInputProperties) (map[string]any, error) {
	props, ok := o.Get()
	if !ok {
		return nil, nil
	}
	out := make(map[string]any, len(props))
	for key, raw := range props {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}
func createTemplateDoc(o externalapi.OptCreateTemplateInputDoc) json.RawMessage {
	return convert.DocJSON(o.Value, o.Set)
}
func updateTemplateDoc(o externalapi.OptUpdateTemplateInputDoc) patch.Field[json.RawMessage] {
	return convert.DocPatch(o)
}
func createBroadcastDoc(o externalapi.OptCreateBroadcastInputDoc) json.RawMessage {
	return convert.DocJSON(o.Value, o.Set)
}
func updateBroadcastDoc(o externalapi.OptUpdateBroadcastInputDoc) patch.Field[json.RawMessage] {
	return convert.DocPatch(o)
}
func stepDocIn(o externalapi.OptAutomationStepDoc) json.RawMessage {
	return convert.DocJSON(o.Value, o.Set)
}

func createContactFields(o externalapi.OptNilCreateContactInputCustomFields) map[string]any {
	v, _ := o.Get()
	return convert.RawMap(v)
}

func upsertContactFields(o externalapi.OptNilUpsertContactInputCustomFields) map[string]any {
	v, _ := o.Get()
	return convert.RawMap(v)
}

func updateContactFields(o externalapi.OptNilUpdateContactInputCustomFields) patch.Field[map[string]any] {
	return convert.MapPatch(o)
}

func createIntegrationParams(o externalapi.OptCreateIntegrationInputParams) map[string]any {
	v, _ := o.Get()
	return convert.RawMap(v)
}

func updateIntegrationParams(o externalapi.OptUpdateIntegrationInputParams) patch.Field[map[string]any] {
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
func integrationConfigIn(in externalapi.IntegrationConfigInput) (integrations.ConfigInput, error) {
	switch in.OneOf.Type {
	case externalapi.SmtpConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SmtpConfigInput
		return integrations.ConfigInput{SMTP: &integrations.SMTPConfig{
			Host: c.Host, Port: int(c.Port), Username: c.Username.Or(""), Password: c.Password.Or(""),
			From: string(c.From), FromName: c.FromName.Or(""),
		}}, nil
	case externalapi.SesConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SesConfigInput
		return integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: c.Region, AccessKeyID: c.AccessKeyId, SecretAccessKey: c.SecretAccessKey,
			From: string(c.From), FromName: c.FromName.Or(""), Endpoint: c.Endpoint.Or(""),
		}}, nil
	case externalapi.InboxConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Inbox: &integrations.InboxConfig{}}, nil
	}
	return integrations.ConfigInput{}, errUnknownProvider
}

func integrationConfigPatch(o externalapi.OptIntegrationConfigInput) (patch.Field[integrations.ConfigInput], error) {
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

func createSegmentCondition(g externalapi.ConditionGroup) (*string, error) {
	def, err := condition.Encode(&g)
	if err != nil {
		return nil, err
	}
	return &def, nil
}

func segmentConditionPatch(o externalapi.OptConditionGroup) (patch.Field[string], error) {
	def, err := condition.EncodeOptional(o)
	if err != nil {
		return patch.Field[string]{}, err
	}
	return patch.FromPtr(def), nil
}
