package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// SPIKE (#1674): a Condition of each place, as a client writes it.
const (
	spikeSegmentCondition = `{"kind":"group","combinator":"and","entries":[
		{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"contains","value":"@acme.test"}},
		{"kind":"tag","name":"vip","operator":"has"},
		{"kind":"group","combinator":"or","not":true,"entries":[
			{"kind":"customField","key":"seats","test":{"kind":"number","operator":"gte","value":2.5}},
			{"kind":"customField","key":"plan","test":{"kind":"presence","operator":"notSet"}},
			{"kind":"customField","key":"renewed","test":{"kind":"moment","operator":"before","at":"2026-01-02T03:04:05Z"}},
			{"kind":"customField","key":"renewed","test":{"kind":"withinLast","days":30}}
		]},
		{"kind":"event","action":"order.placed","operator":"atLeast","times":2,"withinDays":7}
	]}`
	spikeRuleCondition = `{"kind":"group","combinator":"or","entries":[
		{"kind":"tag","name":"vip","operator":"doesNotHave"},
		{"kind":"group","combinator":"and","entries":[
			{"kind":"property","of":{"kind":"trigger"},"key":"cart.total","test":{"kind":"number","operator":"gt","value":100}},
			{"kind":"property","of":{"kind":"trigger"},"key":"coupon","test":{"kind":"presence","operator":"set"}}
		]}
	]}`
	// spikeRuleOnlyLeaf is a property test of the triggering Event: a Rule admits it, a Segment does not.
	spikeRuleOnlyLeaf = `{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}`
)

func TestSiteConditionOfEachPlaceIsStoredAndReadBackEqual(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	var seg siteapi.SegmentConditionInput
	require.NoError(t, seg.UnmarshalJSON([]byte(spikeSegmentCondition)))
	segOut, err := c.SiteConditionSpikeSegment(ctx, &siteapi.SpikeSegmentConditionInput{Condition: seg},
		siteapi.SiteConditionSpikeSegmentParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	segJSON, err := segOut.Condition.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, spikeSegmentCondition, string(segJSON))

	var rule siteapi.RuleConditionInput
	require.NoError(t, rule.UnmarshalJSON([]byte(spikeRuleCondition)))
	ruleOut, err := c.SiteConditionSpikeRule(ctx, &siteapi.SpikeRuleConditionInput{Condition: rule},
		siteapi.SiteConditionSpikeRuleParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	ruleJSON, err := ruleOut.Condition.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, spikeRuleCondition, string(ruleJSON))
}

// A leaf only a Rule admits, nested in a Segment's Condition, is refused at decode with a 400
// naming its path; the same leaf is taken on the Rule's endpoint.
func TestSiteSegmentConditionRefusesARuleOnlyLeafByItsPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	var leaf map[string]any
	require.NoError(t, json.Unmarshal([]byte(spikeRuleOnlyLeaf), &leaf))
	edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		inner := body["condition"].(map[string]any)["entries"].([]any)[2].(map[string]any)
		inner["entries"] = append(inner["entries"].([]any), leaf)
	}))
	var seg siteapi.SegmentConditionInput
	require.NoError(t, seg.UnmarshalJSON([]byte(spikeSegmentCondition)))

	_, err := edited.SiteConditionSpikeSegment(ctx, &siteapi.SpikeSegmentConditionInput{Condition: seg},
		siteapi.SiteConditionSpikeSegmentParams{WorkspaceId: fixtures.AcmePublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Equal(t, []string{"unknown type property"}, refused.Errors.Or(nil)["condition.entries.2.entries.4"])
}
