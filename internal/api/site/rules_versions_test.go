package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteRuleReadsItsLiveVersionAndItsDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got, err := c.SiteRulesGet(context.Background(), siteapi.SiteRulesGetParams{
		WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: siteapi.EntityId(fixtures.RuleTagClickedID),
	})

	require.NoError(t, err)
	res, ok := got.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", got)
	live, isLive := res.Live.Get()
	draft, drafted := res.Draft.Get()
	require.True(t, isLive)
	require.True(t, drafted)
	assert.EqualValues(t, 1, live.Number)
	assert.Equal(t, "rule-clicked", live.Actions[0].Tag.Or(""))
	assert.EqualValues(t, 2, draft.Number)
	assert.Equal(t, "rule-clicked-v2", draft.Actions[0].Tag.Or(""))
	assert.Equal(t, []string{"rule_click"}, draft.TriggerEvents)
}

// The editor sends every field on save, the empty group for no Condition: saving a live Rule
// unchanged makes no draft.
func TestSiteSavingALiveRuleUnchangedMakesNoDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	updated, err := c.SiteRulesUpdate(context.Background(), &siteapi.SiteUpdateRuleInput{
		Name:          siteapi.NewOptString("Either of two events"),
		TriggerEvents: []string{"rule_multi_a", "rule_multi_b"},
		Condition:     siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`)),
		Actions: []siteapi.SiteRuleAction{
			tagAction(siteapi.SiteRuleActionTypeApplyTag, "rule-multi"),
			tagAction(siteapi.SiteRuleActionTypeRemoveTag, "vip"),
		},
	}, siteapi.SiteRulesUpdateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: siteapi.EntityId(fixtures.RuleMultiTriggerID)})

	require.NoError(t, err)
	res, ok := updated.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", updated)
	assert.True(t, res.Draft.Null)
}

func TestSiteRevertRefusesARuleNeverPublished(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got, err := c.SiteRulesRevert(context.Background(), siteapi.SiteRulesRevertParams{
		WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: siteapi.EntityId(fixtures.RuleInactiveID),
	})

	require.NoError(t, err)
	refused, ok := got.(*siteapi.SiteRulesRevertUnprocessableEntity)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, "no unpublished changes to revert", refused.Detail.Or(""))
}
