package external_test

import (
	"context"
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

	got, err := reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleEnrollGreetingID)})
	require.NoError(t, err)
	rule, ok := got.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", got)
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

	got, err := reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleVipOnlyID)})
	require.NoError(t, err)
	rule, ok := got.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", got)
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

	firings, err := reader.RuleFiringsList(ctx, externalapi.RuleFiringsListParams{RuleId: entityIDString(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	listed, ok := firings.(*externalapi.RuleFiringsListOK)
	require.Truef(t, ok, "got %T", firings)
	assert.EqualValues(t, 2, listed.TotalItems)

	foreign, err := reader.RulesGet(ctx, externalapi.RulesGetParams{ID: entityIDString(fixtures.RuleGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesGetNotFound{}, foreign)

	denied := env.ExternalScoped(t, "contacts:read")
	list, err := denied.RulesList(ctx, externalapi.RulesListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesListForbidden{}, list)
}

// Created Rules are always inactive, even for a token that may publish; publishing takes the
// send-class scope, deactivating only the write scope, and a Condition naming an unknown custom
// field is refused.
func TestExternalRulesCreatePublishAndDeactivate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	author := env.ExternalScoped(t, "automations:read", "automations:write")
	publisher := env.ExternalScoped(t, "automations:publish")

	created, err := author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Tag who clicks",
		TriggerEvents: []string{"email.clicked"},
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("clicker")}},
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, externalapi.RuleStatusInactive, res.Status)

	byAuthor, err := author.RulesPublish(ctx, externalapi.RulesPublishParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesPublishForbidden{}, byAuthor)

	published, err := publisher.RulesPublish(ctx, externalapi.RulesPublishParams{ID: res.ID})
	require.NoError(t, err)
	active, ok := published.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", published)
	assert.Equal(t, externalapi.RuleStatusActive, active.Status)

	typo, err := author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Typo",
		TriggerEvents: []string{"email.clicked"},
		Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t,
			`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:no_such_field","operator":"eq","value":"x"}]}`)),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesCreateUnprocessableEntity{}, typo)

	byPublisher, err := publisher.RulesDeactivate(ctx, externalapi.RulesDeactivateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesDeactivateForbidden{}, byPublisher)

	deactivated, err := author.RulesDeactivate(ctx, externalapi.RulesDeactivateParams{ID: res.ID})
	require.NoError(t, err)
	inactive, ok := deactivated.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", deactivated)
	assert.Equal(t, externalapi.RuleStatusInactive, inactive.Status)
}
