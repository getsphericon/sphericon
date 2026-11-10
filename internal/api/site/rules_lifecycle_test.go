package site_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Delete discards a Rule whether or not it has fired (ADR 0100): each leaves the list and is still
// read by id, inactive with discardedAt set, with its Firings, and changes no more; deleting it
// again changes nothing.
func TestSiteRulesDeleteKeepsARuleWithOrWithoutFirings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	fired, never := idStr(fixtures.RuleTagClickedID), idStr(fixtures.RuleInactiveID)

	for _, id := range []siteapi.EntityId{fired, never} {
		err := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: id})
		require.NoError(t, err)

		res, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: id})
		require.NoError(t, err)
		_, discardedAt := res.DiscardedAt.Get()
		assert.True(t, discardedAt)
		assert.Equal(t, siteapi.SiteRuleStatusInactive, res.Status)

		again := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: id})
		require.NoError(t, again)
		_, renamed := c.SiteRulesUpdate(ctx, &siteapi.SiteUpdateRuleInput{Name: siteapi.NewOptName("renamed")},
			siteapi.SiteRulesUpdateParams{WorkspaceId: ws, ID: id})
		testhelper.SiteRefusal(t, renamed, http.StatusNotFound, "a deleted Rule changes no more")
		_, published := c.SiteRulesPublish(ctx, siteapi.SiteRulesPublishParams{WorkspaceId: ws, ID: id})
		testhelper.SiteRefusal(t, published, http.StatusNotFound, "a deleted Rule is never published again")
	}
	firings, err := c.SiteRuleFiringsList(ctx, siteapi.SiteRuleFiringsListParams{WorkspaceId: ws, RuleId: fired})
	require.NoError(t, err)
	assert.NotEmpty(t, firings.Items)

	listed, err := c.SiteRulesList(ctx, siteapi.SiteRulesListParams{WorkspaceId: ws})
	require.NoError(t, err)
	for _, r := range listed.Items {
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

	_, tooMany := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{Name: "big", TriggerEvents: []string{"a"}, Actions: actions},
		siteapi.SiteRulesCreateParams{WorkspaceId: ws})
	refused := testhelper.SiteRefusal(t, tooMany, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "at most 20 Actions")

	leaf := `{"kind":"rule","field":"tag","operator":"has","value":"vip"}`
	var body strings.Builder
	body.WriteString(`{"kind":"group","combinator":"and","rules":[`)
	for i := 0; i <= rules.MaxConditionNodes; i++ {
		body.WriteString(leaf + ",")
	}
	body.WriteString(leaf + `]}`)
	_, bigCondition := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "wide", TriggerEvents: []string{"a"},
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, body.String())),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: ws})
	refusedCondition := testhelper.SiteRefusal(t, bigCondition, http.StatusUnprocessableEntity)
	assert.Contains(t, refusedCondition.Detail.Or(""), "Condition")
}
