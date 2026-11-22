package external_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
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
			{"kind":"customField","key":"courses_completed","test":{"kind":"number","operator":"gte","value":2.5}},
			{"kind":"customField","key":"plan","test":{"kind":"presence","operator":"set"}},
			{"kind":"birthday","test":{"kind":"withinLast","days":30}}
		]},
		{"kind":"event","action":"order.placed","operator":"atLeast","times":2,"withinDays":7}
	]}`
	ruleConditionOfEveryLeaf = `{"kind":"group","combinator":"or","entries":[
		{"kind":"tag","name":"vip","operator":"doesNotHave"},
		{"kind":"group","combinator":"and","entries":[
			{"kind":"property","of":{"kind":"trigger"},"key":"cart.total","test":{"kind":"number","operator":"gt","value":100}},
			{"kind":"property","of":{"kind":"trigger"},"key":"coupon","test":{"kind":"presence","operator":"set"}},
			{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"beginsWith","value":"https://"}}
		]}
	]}`
)

// A Segment's and a Rule's Condition are each stored as sent and read back in the same shape, a
// Rule's in its draft version too.
func TestExternalConditionOfEachPlaceIsReadBackAsSent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeSegmentsRead, apitokens.ScopeSegmentsWrite, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	segment := testhelper.ConditionInput[externalapi.SegmentConditionInput](t, segmentConditionOfEveryLeaf)
	rule := testhelper.ConditionInput[externalapi.RuleConditionInput](t, ruleConditionOfEveryLeaf)

	created, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "Every leaf", Condition: segment})
	require.NoError(t, err)
	updated, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Revision: created.Revision, Condition: externalapi.NewOptSegmentConditionInput(segment)},
		externalapi.SegmentsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	readSegment, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: created.ID})
	require.NoError(t, err)
	ruled, err := c.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name: "Every leaf", TriggerEvents: []string{"checkout"},
		Actions:   []externalapi.RuleActionInput{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("x")}},
		Condition: externalapi.NewOptRuleConditionInput(rule),
	})
	require.NoError(t, err)
	updatedRule, err := c.RulesUpdate(ctx, &externalapi.UpdateRuleInput{Revision: ruled.Revision, Condition: externalapi.NewOptRuleConditionInput(rule)},
		externalapi.RulesUpdateParams{ID: ruled.ID})
	require.NoError(t, err)
	readRule, err := c.RulesGet(ctx, externalapi.RulesGetParams{ID: ruled.ID})
	require.NoError(t, err)
	createdDraft, _ := ruled.Draft.Get()
	updatedDraft, _ := updatedRule.Draft.Get()
	readDraft, _ := readRule.Draft.Get()

	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &created.Condition.Value))
	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &updated.Condition.Value))
	assert.JSONEq(t, segmentConditionOfEveryLeaf, testhelper.JSONOf(t, &readSegment.Condition.Value))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &createdDraft.Condition.Value))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &updatedDraft.Condition.Value))
	assert.JSONEq(t, ruleConditionOfEveryLeaf, testhelper.JSONOf(t, &readDraft.Condition.Value))
}

// A leaf a place does not admit, or a test a leaf does not take, has no value in the place's type:
// the schema refuses it with a 400 that names its path, however deep it sits (ADR 0181).
func TestExternalConditionRefusesWhatItsPlaceDoesNotAdmitByItsPath(t *testing.T) {
	for _, tc := range []struct {
		name, leaf, path string
	}{
		{
			"a property test of the triggering Event in a Segment",
			`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"eq","value":"x"}}`,
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeSegmentsWrite), testhelper.EditBody(func(body map[string]any) {
				body["condition"] = nestedIn(t, tc.leaf)
			}))

			_, err := edited.SegmentsCreate(t.Context(), &externalapi.CreateSegmentInput{
				Name: "Misplaced", Condition: testhelper.ConditionInput[externalapi.SegmentConditionInput](t, `{"kind":"group","combinator":"and","entries":[]}`),
			})

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}

// nestedIn is a Condition holding leaf at entries.1.entries.1, behind a valid leaf at each depth.
func nestedIn(t *testing.T, leaf string) map[string]any {
	t.Helper()
	var condition map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[
		{"kind":"tag","name":"vip","operator":"has"},
		{"kind":"group","combinator":"or","entries":[{"kind":"tag","name":"vip","operator":"has"},`+leaf+`]}
	]}`), &condition))
	return condition
}
