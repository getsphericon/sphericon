package resources

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/invitation"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/signupsurveyresponse"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/brandkit"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/confirmations"
	"github.com/getsphericon/sphericon/internal/consentrecords"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/invitations"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/kernel/tokenscope"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/signup"
	"github.com/getsphericon/sphericon/internal/signupsurvey"
	"github.com/getsphericon/sphericon/internal/templates"
	"github.com/getsphericon/sphericon/internal/usecase/analyticsreport"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// Inputs maps request bodies onto the modules' inputs: the input direction of the contract, so a
// handler only hands the module what the request says. An update becomes the module's patch
// (internal/kernel/patch): an absent key keeps, an explicit null clears, a value sets. The extends below
// bind the generated option types of this surface to the shared conversions of internal/kernel/convert.
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
// goverter:extend nilCountryPatch
// goverter:extend nilLocalePatch
// goverter:extend nilImageURLPatch
// goverter:extend nilDatePatch
// goverter:extend nilEntityIDPatch
// goverter:extend boolPatch
// goverter:extend maxPerSecondPatch
// goverter:extend maxPerDayPatch
// goverter:extend listPatch
// goverter:extend buttonsPtrIn
// goverter:extend buttonsPatch
// goverter:extend stringIn
// goverter:extend emailIn
// goverter:extend stringPtrIn
// goverter:extend nilStringPtrIn
// goverter:extend nilEmailPtrIn
// goverter:extend nilTimeZonePtrIn
// goverter:extend nilCountryPtrIn
// goverter:extend nilLocalePtrIn
// goverter:extend nilImageURLPtrIn
// goverter:extend nilDatePtrIn
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
// goverter:extend signupSourcePtrIn
// goverter:extend customFieldTypePatch
// goverter:extend broadcastContentIn
// goverter:extend broadcastContentPatch
// goverter:extend broadcastChannelPtrIn
// goverter:extend automationStateIn
// goverter:extend automationsSortIn
// goverter:extend sortDirectionIn
type Inputs interface {
	// goverter:ignore Prepared
	TemplateCreate(source *siteapi.CreateTemplateInput) templates.CreateInput
	TemplateUpdate(source *siteapi.UpdateTemplateInput) templates.UpdateInput

	ConsentEvidence(source *siteapi.ConsentEvidence) confirmations.Evidence
	// Where a Broadcast's Test send goes: the address on email, the number on sms.
	TestAddress(source *siteapi.TestSendBroadcastInput) outbound.TestAddress

	BroadcastCreate(source *siteapi.CreateBroadcastInput) (broadcasts.CreateInput, error)
	BroadcastUpdate(source *siteapi.UpdateBroadcastInput) (broadcasts.UpdateInput, error)
	// The arms of a Broadcast's content, which broadcastContentIn and broadcastContentPatch pick.
	EmailBroadcastContentIn(source siteapi.EmailBroadcastContentInput) broadcasts.EmailContentInput
	EmailBroadcastContentPatch(source siteapi.EmailBroadcastContentInput) broadcasts.EmailContentPatch
	TelegramBroadcastContentIn(source siteapi.TelegramBroadcastContentInput) broadcasts.TelegramContentInput
	TelegramBroadcastContentPatch(source siteapi.TelegramBroadcastContentInput) broadcasts.TelegramContentPatch
	SmsBroadcastContentIn(source siteapi.SmsBroadcastContentInput) broadcasts.SMSContentInput
	SmsBroadcastContentPatch(source siteapi.SmsBroadcastContentInput) broadcasts.SMSContentPatch
	// The list's filter; the statuses go member by member (BroadcastStatuses).
	// goverter:map Status Statuses
	BroadcastsListFilter(source siteapi.SiteBroadcastsListParams) broadcasts.ListFilter
	AutomationsListFilter(source siteapi.SiteAutomationsListParams) automations.ListFilter

	WebhookCreate(source *siteapi.CreateWebhookInput) webhooks.CreateInput
	WebhookUpdate(source *siteapi.UpdateWebhookInput) webhooks.UpdateInput

	// goverter:map DkimSelector Selector
	SendingDomainCreate(source *siteapi.CreateSendingDomainInput) sendingdomains.CreateInput

	// goverter:map . Attributes
	ContactCreate(source *siteapi.SiteCreateContactInput) contacts.CreateInput
	ContactAttributes(source siteapi.SiteCreateContactInput) contacts.Attributes
	// goverter:map . Core
	ContactUpdate(source *siteapi.SiteUpdateContactInput) contacts.UpdateInput

	CustomFieldUpdate(source *siteapi.SiteUpdateCustomFieldInput) contacts.CustomFieldUpdateInput

	IntegrationCreate(source *siteapi.CreateIntegrationInput) (integrations.CreateInput, error)
	IntegrationUpdate(source *siteapi.UpdateIntegrationInput) (integrations.UpdateInput, error)

	// goverter:map Condition | createSegmentCondition
	SegmentCreate(source *siteapi.SiteCreateSegmentInput) (segments.CreateInput, error)
	// goverter:map Condition | segmentConditionPatch
	SegmentUpdate(source *siteapi.SiteUpdateSegmentInput) (segments.UpdateInput, error)

	// The editor version is the save's own: the module renders the document and stamps it.
	// goverter:ignore EditorVersion
	AutomationStep(source siteapi.SiteAutomationStep) automationsteps.Step
	AutomationSteps(source []siteapi.SiteAutomationStep) []automationsteps.Step
	TelegramButton(source siteapi.TelegramButton) telegramrender.Button
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
	InvitationCreate(source *siteapi.SiteCreateInvitationInput) invitations.InviteInput

	// Every answer was checked against the contract's enums when the request was decoded.
	SignupSurvey(source *siteapi.SiteSignupSurveyInput) signupsurvey.Answers

	// Enums: goverter checks each member, matched by value (cmd/goverter); a member with no
	// counterpart is mapped with enum:map.
	// The contract names a window "7d", the module counts its days: each member is mapped.
	// goverter:enum:map SiteAnalyticsRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRange90d Window90Days
	AnalyticsWindow(source siteapi.SiteAnalyticsRange) analyticsreport.Window
	// goverter:enum:transform value
	SuppressionChannel(source siteapi.SiteSuppressionChannel) channels.Channel
	// goverter:enum:map SiteAnalyticsRevenueRange7d Window7Days
	// goverter:enum:map SiteAnalyticsRevenueRange30d Window30Days
	// goverter:enum:map SiteAnalyticsRevenueRange90d Window90Days
	AnalyticsRevenueWindow(source siteapi.SiteAnalyticsRevenueRange) analyticsreport.Window
	// The contract names a period "7d", the module counts its days: each member is mapped.
	// goverter:enum:map SiteAutomationReportsGetPeriod7d ReportPeriod7Days
	// goverter:enum:map SiteAutomationReportsGetPeriod14d ReportPeriod14Days
	// goverter:enum:map SiteAutomationReportsGetPeriod30d ReportPeriod30Days
	// goverter:enum:map SiteAutomationReportsGetPeriod90d ReportPeriod90Days
	AutomationReportPeriod(source siteapi.SiteAutomationReportsGetPeriod) automations.ReportPeriod
	// ent's Status has a Default constant beside StatusDraft, so every member is mapped.
	// goverter:enum:map SiteBroadcastStatusDraft StatusDraft
	// goverter:enum:map SiteBroadcastStatusScheduled StatusScheduled
	// goverter:enum:map SiteBroadcastStatusSending StatusSending
	// goverter:enum:map SiteBroadcastStatusSent StatusSent
	// goverter:enum:map SiteBroadcastStatusFailed StatusFailed
	BroadcastStatus(source siteapi.SiteBroadcastStatus) broadcast.Status
	BroadcastStatuses(source []siteapi.SiteBroadcastStatus) []broadcast.Status
	// goverter:enum:transform value
	BroadcastsOrder(source siteapi.SiteBroadcastsOrder) broadcasts.Order
	// goverter:enum:transform value
	BroadcastChannel(source siteapi.BroadcastChannel) channels.Channel
	// goverter:enum:transform value
	AutomationState(source siteapi.SiteAutomationState) automations.State
	// goverter:enum:transform value
	AutomationsSort(source siteapi.SiteAutomationsSort) automations.Sort
	// goverter:enum:transform value
	SortDirection(source siteapi.SortDirection) pagination.Direction
	// goverter:enum:transform value
	AutomationStepType(source siteapi.SiteAutomationStepType) automationsteps.Kind
	// goverter:enum:transform value
	InvitableRole(source siteapi.SiteInvitableRole) invitation.Role
	// goverter:enum:transform value
	MembershipRole(source siteapi.SiteMembershipRole) membership.Role
	// goverter:enum:transform value
	PersonalScope(source siteapi.PersonalScope) tokenscope.PersonalScope
	// goverter:enum:transform value
	RuleActionType(source siteapi.SiteRuleActionType) rules.ActionType
	// goverter:enum:transform value
	SignupCompanySize(source siteapi.SiteSignupCompanySize) signupsurveyresponse.CompanySize
	// goverter:enum:transform value
	SignupGoal(source siteapi.SiteSignupGoal) signupsurveyresponse.MainGoal
	// goverter:enum:transform value
	SignupRole(source siteapi.SiteSignupRole) signupsurveyresponse.Role
	// goverter:enum:transform value
	SignupSource(source siteapi.SiteSignupSource) signupsurveyresponse.HeardFrom
}

func stringPatch(o siteapi.OptString) patch.Field[string] { return convert.Patch(o) }

// customFieldTypePatch is a Custom field's new type as the enum's value: the module reads it as its own.
func customFieldTypePatch(o siteapi.OptSiteCustomFieldType) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilStringPatch(o siteapi.OptNilString) patch.Field[string]      { return convert.Patch(o) }
func nilEmailPatch(o siteapi.OptNilEmailAddress) patch.Field[string] { return convert.StringPatch(o) }
func nilTimeZonePatch(o siteapi.OptNilTimeZoneName) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilCountryPatch(o siteapi.OptNilCountryCode) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilLocalePatch(o siteapi.OptNilLocaleTag) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilImageURLPatch(o siteapi.OptNilImageUrl) patch.Field[string] {
	return convert.StringPatch(o)
}
func nilDatePatch(o siteapi.OptNilDate) patch.Field[time.Time]        { return convert.Patch(o) }
func nilEntityIDPatch(o siteapi.OptNilEntityId) patch.Field[int64]    { return convert.Int64Patch(o) }
func boolPatch(o siteapi.OptBool) patch.Field[bool]                   { return convert.Patch(o) }
func maxPerSecondPatch(o siteapi.OptNilMaxPerSecond) patch.Field[int] { return convert.IntPatch(o) }
func maxPerDayPatch(o siteapi.OptNilMaxPerDay) patch.Field[int]       { return convert.IntPatch(o) }
func listPatch(list []string) patch.Field[[]string]                   { return convert.ListPatch(list) }
func stringIn(o siteapi.OptString) string                             { return o.Or("") }
func emailIn(o siteapi.OptEmailAddress) string                        { return string(o.Or("")) }
func stringPtrIn(o siteapi.OptString) *string                         { return convert.Ptr(o) }
func nilStringPtrIn(o siteapi.OptNilString) *string                   { return convert.Ptr(o) }
func nilEmailPtrIn(o siteapi.OptNilEmailAddress) *string              { return convert.StringPtr(o) }
func nilTimeZonePtrIn(o siteapi.OptNilTimeZoneName) *string           { return convert.StringPtr(o) }
func nilCountryPtrIn(o siteapi.OptNilCountryCode) *string             { return convert.StringPtr(o) }
func nilLocalePtrIn(o siteapi.OptNilLocaleTag) *string                { return convert.StringPtr(o) }
func nilImageURLPtrIn(o siteapi.OptNilImageUrl) *string               { return convert.StringPtr(o) }
func nilDatePtrIn(o siteapi.OptNilDate) *time.Time                    { return convert.Ptr(o) }
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

// buttonsPtrIn is the link buttons a Telegram Broadcast's update replaces: an omitted list (nil)
// keeps the stored ones, an empty list removes them.
func buttonsPtrIn(c Inputs, list []siteapi.TelegramButton) *[]telegramrender.Button {
	if list == nil {
		return nil
	}
	out := make([]telegramrender.Button, len(list))
	for i, b := range list {
		out[i] = c.TelegramButton(b)
	}
	return &out
}

func buttonsPatch(c Inputs, list []siteapi.TelegramButton) patch.Field[[]telegramrender.Button] {
	return patch.FromPtr(buttonsPtrIn(c, list))
}

func timestampPtrIn(o siteapi.OptNilTimestamp) *time.Time {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	t := time.Time(v)
	return &t
}

// signupSourcePtrIn is where the User heard about the product, nil when the request does not say.
func signupSourcePtrIn(c Inputs, o siteapi.OptSiteSignupSource) *signupsurveyresponse.HeardFrom {
	return convert.PtrVia(o, c.SignupSource)
}

// automationStepsPtrIn is the steps an update replaces: omitted (nil) keeps the stored ones.
func automationStepsPtrIn(c Inputs, steps []siteapi.SiteAutomationStep) *[]automationsteps.Step {
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
	case siteapi.TelegramConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Telegram: &integrations.TelegramConfig{Token: in.OneOf.TelegramConfigInput.Token}}, nil
	case siteapi.TwilioConfigInputIntegrationConfigInputSum:
		c := in.OneOf.TwilioConfigInput
		return integrations.ConfigInput{Twilio: &integrations.TwilioConfig{
			AccountSID: c.AccountSid, APIKeySID: c.ApiKeySid.Or(""), Secret: c.Secret, From: c.From,
		}}, nil
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

// errUnknownChannel: content whose channel the contract does not know, which the decoder never
// lets through.
var errUnknownChannel = errors.New("unsupported channel")

// broadcastContentIn maps a Broadcast's content union onto the module's input of its Channel; a
// oneOf is beyond goverter, so only the switch is written here.
func broadcastContentIn(c Inputs, in siteapi.BroadcastContentInput) (broadcasts.ContentInput, error) {
	switch in.OneOf.Type {
	case siteapi.EmailBroadcastContentInputBroadcastContentInputSum:
		return c.EmailBroadcastContentIn(in.OneOf.EmailBroadcastContentInput), nil
	case siteapi.TelegramBroadcastContentInputBroadcastContentInputSum:
		return c.TelegramBroadcastContentIn(in.OneOf.TelegramBroadcastContentInput), nil
	case siteapi.SmsBroadcastContentInputBroadcastContentInputSum:
		return c.SmsBroadcastContentIn(in.OneOf.SmsBroadcastContentInput), nil
	default:
		return nil, errUnknownChannel
	}
}

// broadcastContentPatch is the content of an update: absent keeps the content.
func broadcastContentPatch(c Inputs, o siteapi.OptBroadcastContentInput) (broadcasts.ContentPatch, error) {
	in, ok := o.Get()
	if !ok {
		return nil, nil
	}
	switch in.OneOf.Type {
	case siteapi.EmailBroadcastContentInputBroadcastContentInputSum:
		return c.EmailBroadcastContentPatch(in.OneOf.EmailBroadcastContentInput), nil
	case siteapi.TelegramBroadcastContentInputBroadcastContentInputSum:
		return c.TelegramBroadcastContentPatch(in.OneOf.TelegramBroadcastContentInput), nil
	case siteapi.SmsBroadcastContentInputBroadcastContentInputSum:
		return c.SmsBroadcastContentPatch(in.OneOf.SmsBroadcastContentInput), nil
	default:
		return nil, errUnknownChannel
	}
}

// broadcastChannelPtrIn is the Channel a list is narrowed to: nil for every Channel.
func broadcastChannelPtrIn(c Inputs, o siteapi.OptBroadcastChannel) *channels.Channel {
	v, ok := o.Get()
	if !ok {
		return nil
	}
	return new(c.BroadcastChannel(v))
}

// automationStateIn is the State the Automation list is narrowed to: empty for every State.
func automationStateIn(c Inputs, o siteapi.OptSiteAutomationState) automations.State {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.AutomationState(v)
}

// automationsSortIn is what the Automation list is ordered by: empty for the default.
func automationsSortIn(c Inputs, o siteapi.OptSiteAutomationsSort) automations.Sort {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.AutomationsSort(v)
}

// sortDirectionIn is the direction of a list's order: empty for the list's default.
func sortDirectionIn(c Inputs, o siteapi.OptSortDirection) pagination.Direction {
	v, ok := o.Get()
	if !ok {
		return ""
	}
	return c.SortDirection(v)
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
