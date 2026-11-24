package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func TestGeneratePermissionsDeclaresTheTypeAndAConstantPerValue(t *testing.T) {
	src, err := scopegen.Permissions.Generate([]byte(`{"components":{"schemas":{
	 "Permission":{"type":"string","enum":["members:manage","sending_domains:manage"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package memberships")
	require.Contains(t, out, "type Permission string")
	require.Regexp(t, `PermMembersManage\s+Permission = "members:manage"`, out)
	require.Regexp(t, `PermSendingDomainsManage\s+Permission = "sending_domains:manage"`, out)
}

func TestGenerateTokenScopesListsTheVocabularyInTheContractsOrder(t *testing.T) {
	src, err := scopegen.TokenScopes.Generate([]byte(`{"components":{"schemas":{
	 "ApiTokenScope":{"type":"string","enum":["mcp:send","custom_fields:read"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package apitokens")
	require.Regexp(t, `ScopeMcpSend\s+Scope = "mcp:send"`, out)
	require.Regexp(t, `var Scopes = \[\]Scope\{\s+ScopeMcpSend,\s+ScopeCustomFieldsRead,\s+\}`, out)
}

func TestGenerateCredentialScopesWritesTheServiceVocabularyIntoItsModule(t *testing.T) {
	src, err := scopegen.CredentialScopes.Generate([]byte(`{"components":{"schemas":{
	 "ServiceScope":{"type":"string","enum":["usage:read","holds:write"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package servicecredentials")
	require.NotContains(t, out, "import")
	require.Regexp(t, `ScopeHoldsWrite\s+Scope = "holds:write"`, out)
	require.Regexp(t, `var Scopes = \[\]Scope\{\s+ScopeUsageRead,\s+ScopeHoldsWrite,\s+\}`, out)
}

func TestGenerateAnEnumFileRefusesADocumentWithoutTheEnum(t *testing.T) {
	_, err := scopegen.TokenScopes.Generate([]byte(`{"components":{"schemas":{}}}`))

	require.ErrorContains(t, err, "the document has no ApiTokenScope enum")
}

func TestGenerateUserScopesDeclaresEachEnumOfTheContractInItsOrder(t *testing.T) {
	src, err := scopegen.UserScopes.Generate([]byte(`{"components":{"schemas":{
	 "GrantableScope":{"type":"string","enum":["contacts:read"]},
	 "SendScope":{"type":"string","enum":["mcp:send","emails:send"]},
	 "SetupScope":{"type":"string","enum":["integrations:write"]},
	 "PersonalScope":{"type":"string","enum":["contacts:read","mcp:send","emails:send","integrations:write"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package tokenscope")
	require.NotContains(t, out, "import")
	require.Regexp(t, `GrantableScopeContactsRead\s+GrantableScope = "contacts:read"`, out)
	require.Regexp(t, `var SendScopes = \[\]SendScope\{\s+SendScopeMcpSend,\s+SendScopeEmailsSend,\s+\}`, out)
	require.Regexp(t, `var SetupScopes = \[\]SetupScope\{\s+SetupScopeIntegrationsWrite,\s+\}`, out)
	require.Regexp(t, `var PersonalScopes = \[\]PersonalScope\{\s+PersonalScopeContactsRead,\s+PersonalScopeMcpSend,\s+PersonalScopeEmailsSend,\s+PersonalScopeIntegrationsWrite,\s+\}`, out)
}

func TestGenerateUserScopesRefusesADocumentWithoutOneOfTheEnums(t *testing.T) {
	_, err := scopegen.UserScopes.Generate([]byte(`{"components":{"schemas":{
	 "GrantableScope":{"type":"string","enum":["contacts:read"]}}}}`))

	require.ErrorContains(t, err, "the document has no SendScope enum")
}

func TestGenerateSystemEventActionsDeclaresAnUntypedConstantPerActionAndTheirList(t *testing.T) {
	src, err := scopegen.SystemEventActions.Generate([]byte(`{"components":{"schemas":{
	 "SystemEventAction":{"type":"string","enum":["contact.field_changed","sms.sent"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package systemevent")
	require.NotContains(t, out, "import")
	require.NotRegexp(t, `(?m)^type `, out)
	require.Regexp(t, `ContactFieldChanged\s+= "contact.field_changed"`, out)
	require.Regexp(t, `SmsSent\s+= "sms.sent"`, out)
	require.Regexp(t, `var Actions = \[\]string\{\s+ContactFieldChanged,\s+SmsSent,\s+\}`, out)
}

func TestGenerateConditionKindsListsWhatEachUnionAdmitsInItsOrder(t *testing.T) {
	src, err := scopegen.ConditionKinds.Generate([]byte(`{"components":{"schemas":{
	 "SegmentConditionNode":{"oneOf":[{"$ref":"#/components/schemas/SegmentCondition"},{"$ref":"#/components/schemas/TagLeaf"}],
	  "discriminator":{"propertyName":"kind","mapping":{"tag":"#/components/schemas/TagLeaf","group":"#/components/schemas/SegmentCondition"}}},
	 "RuleConditionNode":{"oneOf":[{"$ref":"#/components/schemas/RuleCondition"},{"$ref":"#/components/schemas/TagLeaf"},{"$ref":"#/components/schemas/RulePropertyLeaf"}],
	  "discriminator":{"propertyName":"kind","mapping":{"property":"#/components/schemas/RulePropertyLeaf","tag":"#/components/schemas/TagLeaf","group":"#/components/schemas/RuleCondition"}}},
	 "EventFilterNode":{"oneOf":[{"$ref":"#/components/schemas/EventFilter"},{"$ref":"#/components/schemas/EventPropertyLeaf"}],
	  "discriminator":{"propertyName":"kind","mapping":{"property":"#/components/schemas/EventPropertyLeaf","group":"#/components/schemas/EventFilter"}}},
	 "AttributeTest":{"oneOf":[{"$ref":"#/components/schemas/TextTest"}],"discriminator":{"propertyName":"kind","mapping":{"text":"#/components/schemas/TextTest"}}},
	 "BirthdayTest":{"oneOf":[{"$ref":"#/components/schemas/WithinLastTest"}],"discriminator":{"propertyName":"kind","mapping":{"withinLast":"#/components/schemas/WithinLastTest"}}},
	 "CustomFieldTest":{"oneOf":[{"$ref":"#/components/schemas/TextTest"},{"$ref":"#/components/schemas/WithinLastTest"}],
	  "discriminator":{"propertyName":"kind","mapping":{"withinLast":"#/components/schemas/WithinLastTest","text":"#/components/schemas/TextTest"}}},
	 "PropertyTest":{"oneOf":[{"$ref":"#/components/schemas/TextTest"}],"discriminator":{"propertyName":"kind","mapping":{"text":"#/components/schemas/TextTest"}}},
	 "EngagementTrackingTest":{"oneOf":[{"$ref":"#/components/schemas/PresenceTest"}],"discriminator":{"propertyName":"kind","mapping":{"presence":"#/components/schemas/PresenceTest"}}},
	 "RulePropertySource":{"oneOf":[{"$ref":"#/components/schemas/TriggerSource"}],"discriminator":{"propertyName":"kind","mapping":{"trigger":"#/components/schemas/TriggerSource"}}}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package conditiontree")
	require.Contains(t, out, "type Kind string")
	require.Regexp(t, `KindGroup\s+Kind = "group"`, out)
	require.Regexp(t, `KindProperty\s+Kind = "property"`, out)
	require.Regexp(t, `var SegmentKinds = \[\]Kind\{\s+KindGroup,\s+KindTag,\s+\}`, out)
	require.Regexp(t, `var RuleKinds = \[\]Kind\{\s+KindGroup,\s+KindTag,\s+KindProperty,\s+\}`, out)
	require.Regexp(t, `var EventFilterKinds = \[\]Kind\{\s+KindGroup,\s+KindProperty,\s+\}`, out)
	require.Regexp(t, `var CustomFieldTests = \[\]TestKind\{\s+TestText,\s+TestWithinLast,\s+\}`, out)
	require.Regexp(t, `var EngagementTrackingTests = \[\]TestKind\{\s+TestPresence,\s+\}`, out)
	require.Regexp(t, `var RuleSources = \[\]Source\{\s+SourceTrigger,\s+\}`, out)
}

func TestGenerateConditionKindsRefusesAUnionMemberWithoutAValue(t *testing.T) {
	_, err := scopegen.ConditionKinds.Generate([]byte(`{"components":{"schemas":{
	 "SegmentConditionNode":{"oneOf":[{"$ref":"#/components/schemas/TagLeaf"}],"discriminator":{"propertyName":"kind","mapping":{}}}}}}`))

	require.ErrorContains(t, err, "SegmentConditionNode: the member #/components/schemas/TagLeaf has no discriminator value")
}

func TestGenerateConditionOperatorsTypesEachOperatorEnumWithItsList(t *testing.T) {
	src, err := scopegen.ConditionOperators.Generate([]byte(`{"components":{"schemas":{
	 "TextOperator":{"type":"string","enum":["eq","beginsWith"]},
	 "NumberOperator":{"type":"string","enum":["gt"]},
	 "MomentOperator":{"type":"string","enum":["before"]},
	 "PresenceOperator":{"type":"string","enum":["set","notSet"]},
	 "HasOperator":{"type":"string","enum":["has","doesNotHave"]},
	 "EventCountOperator":{"type":"string","enum":["atLeast","atMost"]},
	 "EngagementTrackingChannel":{"type":"string","enum":["email","telegram"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Regexp(t, `TextBeginsWith\s+TextOperator = "beginsWith"`, out)
	require.Regexp(t, `var TextOperators = \[\]TextOperator\{\s+TextEq,\s+TextBeginsWith,\s+\}`, out)
	require.Regexp(t, `PresenceNotSet\s+PresenceOperator = "notSet"`, out)
	require.Regexp(t, `DoesNotHave\s+HasOperator = "doesNotHave"`, out)
	require.Regexp(t, `AtMost\s+EventCountOperator = "atMost"`, out)
	require.Regexp(t, `var TrackingChannels = \[\]TrackingChannel\{\s+TrackingEmail,\s+TrackingTelegram,\s+\}`, out)
}
