package site_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Delete removes a Rule whether or not it has fired. One that never fired is gone; one with
// Firings leaves the list and is still read by id, inactive with deletedAt set, with its Firings
// (ADR 0098), and deleting it again changes nothing.
func TestSiteRulesDeleteRemovesARuleWithOrWithoutFirings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	fired, never := idStr(fixtures.RuleTagClickedID), idStr(fixtures.RuleInactiveID)

	for _, id := range []siteapi.EntityId{fired, never} {
		deleted, err := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: id})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteRulesDeleteNoContent{}, deleted)
	}

	gone, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: never})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesGetNotFound{}, gone)
	kept, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: fired})
	require.NoError(t, err)
	res, ok := kept.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", kept)
	_, deletedAt := res.DeletedAt.Get()
	assert.True(t, deletedAt)
	assert.Equal(t, siteapi.SiteRuleStatusInactive, res.Status)
	firings, err := c.SiteRuleFiringsList(ctx, siteapi.SiteRuleFiringsListParams{WorkspaceId: ws, RuleId: fired})
	require.NoError(t, err)
	assert.NotEmpty(t, firings.(*siteapi.SiteRuleFiringsListOK).Items)
	again, err := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: fired})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesDeleteNoContent{}, again)
	renamed, err := c.SiteRulesUpdate(ctx, &siteapi.SiteUpdateRuleInput{Name: siteapi.NewOptString("renamed")},
		siteapi.SiteRulesUpdateParams{WorkspaceId: ws, ID: fired})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesUpdateNotFound{}, renamed, "a deleted Rule changes no more")

	listed, err := c.SiteRulesList(ctx, siteapi.SiteRulesListParams{WorkspaceId: ws})
	require.NoError(t, err)
	for _, r := range listed.(*siteapi.SiteRulesListOK).Items {
		assert.NotContains(t, []siteapi.EntityId{fired, never}, r.ID)
	}
}

func TestSiteRulesRefuseTooManyActionsAndAnOversizedCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	actions := make([]siteapi.SiteRuleAction, rules.MaxActions+1)
	for i := range actions {
		actions[i] = tagAction(siteapi.SiteRuleActionTypeApplyTag, "t"+strconv.Itoa(i))
	}

	tooMany, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{Name: "big", TriggerEvents: []string{"a"}, Actions: actions},
		siteapi.SiteRulesCreateParams{WorkspaceId: ws})
	require.NoError(t, err)
	refused, ok := tooMany.(*siteapi.SiteRulesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", tooMany)
	assert.Contains(t, refused.Detail.Or(""), "at most 20 Actions")

	leaf := `{"kind":"rule","field":"tag","operator":"has","value":"vip"}`
	var body strings.Builder
	body.WriteString(`{"kind":"group","combinator":"and","rules":[`)
	for i := 0; i <= rules.MaxConditionNodes; i++ {
		body.WriteString(leaf + ",")
	}
	body.WriteString(leaf + `]}`)
	bigCondition, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "wide", TriggerEvents: []string{"a"},
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, body.String())),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: ws})
	require.NoError(t, err)
	refusedCondition, ok := bigCondition.(*siteapi.SiteRulesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", bigCondition)
	assert.Contains(t, refusedCondition.Detail.Or(""), "Condition")
}
