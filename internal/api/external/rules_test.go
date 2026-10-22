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
	assert.Equal(t, []string{"rule_vip"}, rule.TriggerEvents)
	condition, hasCondition := rule.Condition.Get()
	require.True(t, hasCondition)
	assert.Len(t, condition.Rules, 1)
	require.Len(t, rule.Actions, 1)
	assert.Equal(t, "rule-vip-seen", rule.Actions[0].Tag.Or(""))

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
	assert.IsType(t, &externalapi.RulesListUnauthorized{}, list)
}

// Created Rules are always drafts, even for a token that may activate; activating takes the
// send-class scope, and a Condition naming an unknown custom field is refused.
func TestExternalRulesCreateAndActivate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	author := env.ExternalScoped(t, "automations:read", "automations:write")
	activator := env.ExternalScoped(t, "automations:activate")

	created, err := author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Tag who clicks",
		TriggerEvents: []string{"email.clicked"},
		Actions:       []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("clicker")}},
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, externalapi.RuleStatusDraft, res.Status)

	byAuthor, err := author.RulesActivate(ctx, externalapi.RulesActivateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesActivateUnauthorized{}, byAuthor)

	activated, err := activator.RulesActivate(ctx, externalapi.RulesActivateParams{ID: res.ID})
	require.NoError(t, err)
	active, ok := activated.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", activated)
	assert.Equal(t, externalapi.RuleStatusActive, active.Status)

	typo, err := author.RulesCreate(ctx, &externalapi.CreateRuleInput{
		Name:          "Typo",
		TriggerEvents: []string{"email.clicked"},
		Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t,
			`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:no_such_field","operator":"eq","value":"x"}]}`)),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.RulesCreateUnprocessableEntity{}, typo)

	deactivated, err := activator.RulesDeactivate(ctx, externalapi.RulesDeactivateParams{ID: res.ID})
	require.NoError(t, err)
	draft, ok := deactivated.(*externalapi.RuleResource)
	require.Truef(t, ok, "got %T", deactivated)
	assert.Equal(t, externalapi.RuleStatusDraft, draft.Status)
}
