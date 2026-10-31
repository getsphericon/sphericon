package resources

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent/invitation"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/signup"
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
// goverter:extend int32ToInt
// goverter:extend listPtrIn
// goverter:extend timestampPtrIn
// goverter:extend automationStepsPtrIn
// goverter:extend ruleActionsPtrIn
// goverter:extend documentIn
// goverter:extend documentPatch
// goverter:extend objectIn
// goverter:extend objectPatch
// goverter:extend nilObjectIn
// goverter:extend nilObjectPatch
type Inputs interface {
	// goverter:ignore Prepared
	TemplateCreate(source *siteapi.CreateTemplateInput) templates.CreateInput
	TemplateUpdate(source *siteapi.UpdateTemplateInput) templates.UpdateInput

	BroadcastCreate(source *siteapi.CreateBroadcastInput) broadcasts.CreateInput
	BroadcastUpdate(source *siteapi.UpdateBroadcastInput) broadcasts.UpdateInput

	WebhookCreate(source *siteapi.CreateWebhookInput) webhooks.CreateInput
	WebhookUpdate(source *siteapi.UpdateWebhookInput) webhooks.UpdateInput

	// goverter:map DkimSelector Selector
	SendingDomainCreate(source *siteapi.CreateSendingDomainInput) sendingdomains.CreateInput

	// goverter:map . Attributes
	ContactCreate(source *siteapi.SiteCreateContactInput) contacts.CreateInput
	ContactAttributes(source siteapi.SiteCreateContactInput) contacts.Attributes
	ContactUpdate(source *siteapi.SiteUpdateContactInput) contacts.UpdateInput

	IntegrationCreate(source *siteapi.CreateIntegrationInput) (integrations.CreateInput, error)
	IntegrationUpdate(source *siteapi.UpdateIntegrationInput) (integrations.UpdateInput, error)

	// goverter:map Condition | createSegmentCondition
	SegmentCreate(source *siteapi.SiteCreateSegmentInput) (segments.CreateInput, error)
	// goverter:map Condition | segmentConditionPatch
	SegmentUpdate(source *siteapi.SiteUpdateSegmentInput) (segments.UpdateInput, error)

	// The editor version is the save's own: the module renders the document and stamps it.
	// goverter:ignore EditorVersion
	AutomationStep(source siteapi.SiteAutomationStep) automations.Step
	AutomationSteps(source []siteapi.SiteAutomationStep) []automations.Step
	AutomationCreate(source *siteapi.SiteCreateAutomationInput) automations.CreateInput
	AutomationUpdate(source *siteapi.SiteUpdateAutomationInput) automations.UpdateInput

	RuleActions(source []siteapi.SiteRuleAction) []rules.Action
	// goverter:map Condition | ruleConditionIn
	RuleCreate(source *siteapi.SiteCreateRuleInput) (rules.CreateInput, error)
	// goverter:map Condition | ruleConditionIn
	RuleUpdate(source *siteapi.SiteUpdateRuleInput) (rules.UpdateInput, error)

	TokenCreate(source *siteapi.SiteCreateTokenInput) apitokens.Input
	PersonalTokenCreate(source *siteapi.SiteCreatePersonalTokenInput) apitokens.PersonalInput

	BrandKitSave(source *siteapi.SiteSaveBrandKitInput) brandkit.Input

	SignUp(source *siteapi.SiteSignUpInput) signup.Input
	SignupAnswers(source []siteapi.SiteSignupItemAnswer) []consentrecords.Answer
	ReAcceptances(source []siteapi.SiteAcceptance) []consentrecords.ReAcceptance

	// The inviting User is the session's, not the request's: the handler sets it.
	// goverter:ignore InvitedBy
	InvitationCreate(source *siteapi.SiteCreateInvitationInput) accounts.InviteInput

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a window "7d", the module counts its days: each member is mapped.
	// goverter:enum:map SiteAnalyticsRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRange90d Window90Days
	AnalyticsWindow(source siteapi.SiteAnalyticsRange) analytics.Window
	// goverter:enum:map SiteAnalyticsRevenueRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRevenueRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRevenueRange90d Window90Days
	AnalyticsRevenueWindow(source siteapi.SiteAnalyticsRevenueRange) analytics.Window
	// goverter:enum:transform value
	AutomationStepType(source siteapi.SiteAutomationStepType) automations.StepType
	// goverter:enum:transform value
	InvitableRole(source siteapi.SiteInvitableRole) invitation.Role
	// goverter:enum:transform value
	RuleActionType(source siteapi.SiteRuleActionType) rules.ActionType
}

func stringPatch(o siteapi.OptString) patch.Field[string]            { return convert.Patch(o) }
func nilStringPatch(o siteapi.OptNilString) patch.Field[string]      { return convert.Patch(o) }
func nilEmailPatch(o siteapi.OptNilEmailAddress) patch.Field[string] { return convert.StringPatch(o) }
func nilTimeZonePatch(o siteapi.OptNilTimeZoneName) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilEntityIDPatch(o siteapi.OptNilEntityId) patch.Field[int64]    { return convert.Int64Patch(o) }
func boolPatch(o siteapi.OptBool) patch.Field[bool]                   { return convert.Patch(o) }
func maxPerSecondPatch(o siteapi.OptNilMaxPerSecond) patch.Field[int] { return convert.IntPatch(o) }
func maxPerDayPatch(o siteapi.OptNilMaxPerDay) patch.Field[int]       { return convert.IntPatch(o) }
func listPatch(list []string) patch.Field[[]string]                   { return convert.ListPatch(list) }
func stringIn(o siteapi.OptString) string                             { return o.Or("") }
func stringPtrIn(o siteapi.OptString) *string                         { return convert.Ptr(o) }
func nilStringPtrIn(o siteapi.OptNilString) *string                   { return convert.Ptr(o) }
func nilEmailPtrIn(o siteapi.OptNilEmailAddress) *string              { return convert.StringPtr(o) }
func nilTimeZonePtrIn(o siteapi.OptNilTimeZoneName) *string           { return convert.StringPtr(o) }
func nilEntityIDPtrIn(o siteapi.OptNilEntityId) *int64                { return convert.Int64Ptr(o) }
func boolIn(o siteapi.OptBool) bool                                   { return o.Or(false) }
func boolPtrIn(o siteapi.OptBool) *bool                               { return convert.Ptr(o) }
func int32In(o siteapi.OptInt32) int                                  { return int(o.Or(0)) }
func entityIDIn(o siteapi.OptEntityId) int64                          { return int64(o.Or(0)) }
func maxPerSecondPtrIn(o siteapi.OptNilMaxPerSecond) *int             { return intPtr(o) }
func maxPerDayPtrIn(o siteapi.OptNilMaxPerDay) *int                   { return intPtr(o) }
func int32ToInt(v int32) int                                          { return int(v) }

// listPtrIn is a list an update replaces: an omitted list (nil) keeps the stored one.
func listPtrIn(list []string) *[]string {
	if list == nil {
		return nil
	}
	return &list
}

func timestampPtrIn(o siteapi.OptNilTimestamp) *time.Time {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	t := time.Time(v)
	return &t
}

// automationStepsPtrIn is the steps an update replaces: omitted (nil) keeps the stored ones.
func automationStepsPtrIn(c Inputs, steps []siteapi.SiteAutomationStep) *[]automations.Step {
	if steps == nil {
		return nil
	}
	out := c.AutomationSteps(steps)
	return &out
}

// ruleActionsPtrIn is the Actions an update replaces: omitted (nil) keeps the stored ones.
func ruleActionsPtrIn(c Inputs, actions []siteapi.SiteRuleAction) *[]rules.Action {
	if actions == nil {
		return nil
	}
	out := c.RuleActions(actions)
	return &out
}

func ruleConditionIn(o siteapi.OptSiteConditionGroup) (*string, error) {
	return condition.EncodeOptional(o)
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
		}}, nil
	case siteapi.InboxConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Inbox: &integrations.InboxConfig{}}, nil
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

func createSegmentCondition(g siteapi.SiteConditionGroup) (*string, error) {
	def, err := condition.Encode(&g)
	if err != nil {
		return nil, err
	}
	return &def, nil
}

func segmentConditionPatch(o siteapi.OptSiteConditionGroup) (patch.Field[string], error) {
	def, err := condition.EncodeOptional(o)
	if err != nil {
		return patch.Field[string]{}, err
	}
	return patch.FromPtr(def), nil
}
