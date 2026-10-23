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

func TestSiteRulesArchiveHidesARuleAndUnarchiveBringsItBackAsADraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := idStr(fixtures.RuleTagClickedID)

	archived, err := c.SiteRulesArchive(ctx, siteapi.SiteRulesArchiveParams{WorkspaceId: ws, ID: id})
	require.NoError(t, err)
	res, ok := archived.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", archived)
	_, isArchived := res.ArchivedAt.Get()
	assert.True(t, isArchived)
	assert.Equal(t, siteapi.SiteRuleStatusDraft, res.Status)

	kept, err := c.SiteRulesList(ctx, siteapi.SiteRulesListParams{WorkspaceId: ws})
	require.NoError(t, err)
	for _, r := range kept.(*siteapi.SiteRulesListOK).Items {
		assert.NotEqual(t, id, r.ID)
	}
	listed, err := c.SiteRulesList(ctx, siteapi.SiteRulesListParams{WorkspaceId: ws, Archived: siteapi.NewOptBool(true)})
	require.NoError(t, err)
	require.Len(t, listed.(*siteapi.SiteRulesListOK).Items, 1)

	act, err := c.SiteRulesActivate(ctx, siteapi.SiteRulesActivateParams{WorkspaceId: ws, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesActivateNotFound{}, act, "an archived Rule is restored before it is activated")

	restored, err := c.SiteRulesUnarchive(ctx, siteapi.SiteRulesUnarchiveParams{WorkspaceId: ws, ID: id})
	require.NoError(t, err)
	back, ok := restored.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", restored)
	_, isArchived = back.ArchivedAt.Get()
	assert.False(t, isArchived)
	assert.Equal(t, siteapi.SiteRuleStatusDraft, back.Status)
}

func TestSiteRulesDeletePurgesARuleWithNoFiringsAndRefusesOneWithFirings(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	refused, err := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.RuleTagClickedID)})
	require.NoError(t, err)
	problem, ok := refused.(*siteapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", refused)
	require.Len(t, problem.UsedBy, 1)
	assert.Equal(t, siteapi.UsedByKindFiring, problem.UsedBy[0].Kind)
	assert.Equal(t, fixtures.RuleTagClickedName, problem.UsedBy[0].Name)
	assert.EqualValues(t, 2, problem.UsedBy[0].Count.Or(0))

	deleted, err := c.SiteRulesDelete(ctx, siteapi.SiteRulesDeleteParams{WorkspaceId: ws, ID: idStr(fixtures.RuleDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesDeleteNoContent{}, deleted)
	gone, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: ws, ID: idStr(fixtures.RuleDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesGetNotFound{}, gone)
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
