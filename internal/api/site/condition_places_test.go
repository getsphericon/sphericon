package site_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Condition of each place (ADR 0181), as a client writes it: every kind of leaf and test the place
// admits, nested and negated.
const (
	segmentConditionOfEveryLeaf = `{"kind":"group","combinator":"and","entries":[
		{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"contains","value":"@acme.test"}},
		{"kind":"attribute","attribute":"phone","test":{"kind":"presence","operator":"set"}},
		{"kind":"birthday","test":{"kind":"moment","operator":"before","at":"2000-01-02T03:04:05Z"}},
		{"kind":"tag","name":"vip","operator":"has"},
		{"kind":"group","combinator":"or","not":true,"entries":[
			{"kind":"telegram","operator":"doesNotHave"},
			{"kind":"sms","operator":"has"},
			{"kind":"engagementTracking","channel":"telegram","test":{"kind":"presence","operator":"notSet"}},
			{"kind":"engagementTracking","channel":"email","test":{"kind":"text","operator":"eq","value":"denied"}},
			{"kind":"customField","key":"courses_completed","test":{"kind":"number","operator":"gte","value":2.5}},
			{"kind":"customField","key":"plan","test":{"kind":"presence","operator":"set"}},
			{"kind":"birthday","test":{"kind":"withinLast","days":30}}
		]},
		{"kind":"event","action":"order.placed","operator":"atLeast","times":2,"withinDays":7}
	]}`
	ruleConditionOfEveryLeaf = `{"kind":"group","combinator":"or","entries":[
		{"kind":"tag","name":"vip","operator":"doesNotHave"},
		{"kind":"engagementTracking","channel":"sms","test":{"kind":"text","operator":"neq","value":"granted"}},
		{"kind":"group","combinator":"and","entries":[
			{"kind":"property","of":{"kind":"trigger"},"key":"cart.total","test":{"kind":"number","operator":"gt","value":100}},
			{"kind":"property","of":{"kind":"trigger"},"key":"coupon","test":{"kind":"presence","operator":"set"}},
			{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"beginsWith","value":"https://"}}
		]}
	]}`
)

// A Segment's and a Rule's Condition are each stored as sent and read back in the same shape, a
// Rule's in its draft version too.
func TestSiteConditionOfEachPlaceIsReadBackAsSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	segment := testhelper.ConditionInput[siteapi.SegmentConditionInput](t, segmentConditionOfEveryLeaf)
	rule := testhelper.ConditionInput[siteapi.RuleConditionInput](t, ruleConditionOfEveryLeaf)

	created, err := c.SiteSegmentsCreate(ctx, &siteapi.CreateSegmentInput{Name: "Every leaf", Condition: segment},
		siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	updated, err := c.SiteSegmentsUpdate(ctx, &siteapi.UpdateSegmentInput{Revision: created.Revision, Condition: siteapi.NewOptSegmentConditionInput(segment)},
		siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	readSegment, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	ruled, err := c.SiteRulesCreate(ctx, &siteapi.CreateRuleInput{
		Name: "Every leaf", TriggerEvents: []string{"checkout"},
		Actions:   []siteapi.RuleActionInput{tagAction(siteapi.RuleActionTypeApplyTag, "x")},
		Condition: siteapi.NewOptRuleConditionInput(rule),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	updatedRule, err := c.SiteRulesUpdate(ctx, &siteapi.UpdateRuleInput{Revision: ruled.Revision, Condition: siteapi.NewOptRuleConditionInput(rule)},
		siteapi.SiteRulesUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: ruled.ID})
	require.NoError(t, err)
	readRule, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: fixtures.AcmePublicID, ID: ruled.ID})
	require.NoError(t, err)
	createdDraft, _ := ruled.Draft.Get()
	updatedDraft, _ := updatedRule.Draft.Get()
	readDraft, _ := readRule.Draft.Get()

	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &created.Condition))
	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &updated.Condition))
	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &readSegment.Condition))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &createdDraft.Condition))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &updatedDraft.Condition))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &readDraft.Condition))
}

// A leaf a place does not admit, or a test a leaf does not take, has no value in the place's type:
// the schema refuses it with a 400 that names its path, however deep it sits (ADR 0181).
func TestSiteConditionRefusesWhatItsPlaceDoesNotAdmitByItsPath(t *testing.T) {
	for _, tc := range []struct {
		name, leaf, path string
	}{
		{
			"a property test of the triggering Event in a Segment",
			`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"eq","value":"x"}}`,
			"condition.entries.1.entries.1",
		},
		{
			"a test of an Enrollment's own message in a Segment",
			`{"kind":"stepMessage","step":"5a0f6d2e-8b1c-4e3a-9f7d-000000000002","engagement":"opened"}`,
			"condition.entries.1.entries.1",
		},
		{
			"a text operator on a number test",
			`{"kind":"customField","key":"courses_completed","test":{"kind":"number","operator":"contains","value":1}}`,
			"condition.entries.1.entries.1.test.operator",
		},
		{
			"an unknown attribute",
			`{"kind":"attribute","attribute":"password","test":{"kind":"text","operator":"eq","value":"x"}}`,
			"condition.entries.1.entries.1.attribute",
		},
		{
			"a number test on an attribute",
			`{"kind":"attribute","attribute":"email","test":{"kind":"number","operator":"gt","value":1}}`,
			"condition.entries.1.entries.1.test",
		},
		{
			"a consent of a Channel that takes none",
			`{"kind":"engagementTracking","channel":"push","test":{"kind":"presence","operator":"set"}}`,
			"condition.entries.1.entries.1.channel",
		},
		{
			"a number test on a consent",
			`{"kind":"engagementTracking","channel":"email","test":{"kind":"number","operator":"gt","value":1}}`,
			"condition.entries.1.entries.1.test",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
				body["condition"] = testhelper.NestedCondition(t, tc.leaf)
			}))

			_, err := edited.SiteSegmentsCreate(t.Context(), &siteapi.CreateSegmentInput{
				Name: "Misplaced", Condition: testhelper.ConditionInput[siteapi.SegmentConditionInput](t, `{"kind":"group","combinator":"and","entries":[]}`),
			}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})

			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}

// A Rule's Condition does not admit the test of an Enrollment's own message, which is a Branch
// path's alone (ADR 0122): the schema refuses it with a 400 that names its path.
func TestSiteRuleConditionRefusesATestOfAnEnrollmentsOwnMessage(t *testing.T) {
	env := testhelper.Setup(t)
	edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		body["condition"] = testhelper.NestedCondition(t, `{"kind":"stepMessage","step":"5a0f6d2e-8b1c-4e3a-9f7d-000000000002","engagement":"clicked"}`)
	}))

	_, err := edited.SiteRulesCreate(t.Context(), &siteapi.CreateRuleInput{
		Name: "Misplaced", TriggerEvents: []string{"checkout"},
		Actions:   []siteapi.RuleActionInput{tagAction(siteapi.RuleActionTypeApplyTag, "x")},
		Condition: siteapi.NewOptRuleConditionInput(testhelper.ConditionInput[siteapi.RuleConditionInput](t, `{"kind":"group","combinator":"and","entries":[]}`)),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: fixtures.AcmePublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Equal(t, []string{"condition.entries.1.entries.1"}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
}

// Every attribute the contract lists is one a Condition tests: none is refused.
func TestSiteConditionTestsEveryContactAttribute(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	for _, attribute := range siteapi.ContactAttribute("").AllValues() {
		_, err := c.SiteSegmentsPreview(t.Context(), &siteapi.SitePreviewSegmentInput{
			Condition: siteapi.NewOptSegmentConditionInput(testhelper.ConditionInput[siteapi.SegmentConditionInput](t,
				`{"kind":"group","combinator":"and","entries":[{"kind":"attribute","attribute":"`+string(attribute)+`","test":{"kind":"presence","operator":"set"}}]}`)),
		}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.AcmePublicID})

		require.NoError(t, err, attribute)
	}
}
