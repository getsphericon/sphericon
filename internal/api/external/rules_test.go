package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Rule that enrolls Contacts in an Automation reads as it is stored: the action kind and the
// Automation it names, the same as on the site API.
func TestExternalRulesReadAnEnrollAction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "automations:read")

	rule, err := reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleEnrollGreetingID)})
	require.NoError(t, err)
	live, published := rule.Live.Get()
	require.True(t, published)
	require.Len(t, live.Actions, 1)
	assert.Equal(t, externalapi.RuleActionTypeEnrollAutomation, live.Actions[0].Type)
	assert.Equal(t, entityIDString(fixtures.AutomationJobsGreetingID), live.Actions[0].AutomationId.Or(0))
}

func TestExternalRulesReadNeedsTheAutomationsReadScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	reader := env.ExternalScoped(t, "automations:read")

	rule, err := reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleVipOnlyID)})
	require.NoError(t, err)
	assert.Equal(t, fixtures.RuleVipOnlyName, rule.Name)
	assert.Equal(t, externalapi.RuleStatusActive, rule.Status)
	live, published := rule.Live.Get()
	require.True(t, published)
	assert.True(t, rule.Draft.Null, "no unpublished changes")
	assert.EqualValues(t, 1, live.Number)
	assert.Equal(t, []string{"rule_vip"}, live.TriggerEvents)
	condition, hasCondition := live.Condition.Get()
	require.True(t, hasCondition)
	assert.Len(t, condition.Rules, 1)
	require.Len(t, live.Actions, 1)
	assert.Equal(t, "rule-vip-seen", live.Actions[0].Tag.Or(""))

	listed, err := reader.RuleFiringsList(ctx, externalapi.RuleFiringsListParams{RuleId: entityIDString(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	assert.EqualValues(t, 2, listed.TotalItems)

	_, err = reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	denied := env.ExternalScoped(t, "contacts:read")
	_, err = denied.RulesList(ctx, externalapi.RulesListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// Created Rules are always inactive, even for a token that may publish; publishing takes the
// send-class scope, deactivating only the write scope, and a Condition naming an unknown custom
// field is refused.
func TestExternalRulesCreatePublishAndDeactivate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	author := env.ExternalScoped(t, "automations:read", "automations:write")
	publisher := env.ExternalScoped(t, "automations:publish")

	res, err := author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Tag who clicks",
		TriggerEvents: []string{"email.clicked"},
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("clicker")}},
	})
	require.NoError(t, err)
	assert.Equal(t, externalapi.RuleStatusInactive, res.Status)

	_, err = author.RulesPublish(ctx, externalapi.RulesPublishParams{ID: res.ID})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	active, err := publisher.RulesPublish(ctx, externalapi.RulesPublishParams{ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, externalapi.RuleStatusActive, active.Status)

	_, err = author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Typo",
		TriggerEvents: []string{"email.clicked"},
		Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t,
			`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:no_such_field","operator":"eq","value":"x"}]}`)),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Clicked twice this week",
		TriggerEvents: []string{"email.clicked"},
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("keen")}},
		Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t,
			`{"kind":"group","combinator":"and","rules":[{"kind":"event","action":"email.clicked","operator":"atLeast","times":2,"withinDays":7}]}`)),
	})
	require.NoError(t, err)

	_, err = publisher.RulesDeactivate(ctx, externalapi.RulesDeactivateParams{ID: res.ID})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	inactive, err := author.RulesDeactivate(ctx, externalapi.RulesDeactivateParams{ID: res.ID})
	require.NoError(t, err)
	assert.Equal(t, externalapi.RuleStatusInactive, inactive.Status)
}
