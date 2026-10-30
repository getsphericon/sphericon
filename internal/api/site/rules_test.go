package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const vipCondition = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"vip"}]}`

func tagAction(kind siteapi.SiteRuleActionType, name string) siteapi.SiteRuleAction {
	return siteapi.SiteRuleAction{Type: kind, Tag: siteapi.NewOptString(name)}
}

func TestSiteRulesCRUDPublishAndDeactivate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name:          "Tag the vips who clicked",
		TriggerEvents: []string{"email.clicked", "link_followed", "email.clicked"},
		Condition:     siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, vipCondition)),
		Actions:       []siteapi.SiteRuleAction{tagAction(siteapi.SiteRuleActionTypeApplyTag, "clicker"), tagAction(siteapi.SiteRuleActionTypeRemoveTag, "cold")},
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, siteapi.SiteRuleStatusInactive, res.Status)
	assert.True(t, res.Live.Null, "a new Rule is not live")
	draft, drafted := res.Draft.Get()
	require.True(t, drafted)
	assert.EqualValues(t, 1, draft.Number)
	assert.Equal(t, []string{"email.clicked", "link_followed"}, draft.TriggerEvents, "a repeated Trigger event is kept once")
	require.Len(t, draft.Actions, 2)
	assert.Equal(t, siteapi.SiteRuleActionTypeApplyTag, draft.Actions[0].Type)
	assert.Equal(t, "clicker", draft.Actions[0].Tag.Or(""))
	assert.Equal(t, siteapi.SiteRuleActionTypeRemoveTag, draft.Actions[1].Type)
	condition, hasCondition := draft.Condition.Get()
	require.True(t, hasCondition)
	assert.Len(t, condition.Rules, 1)
	_, activated := res.ActivatedAt.Get()
	assert.False(t, activated)

	act, err := c.SiteRulesPublish(ctx, siteapi.SiteRulesPublishParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	actRes, ok := act.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", act)
	assert.Equal(t, siteapi.SiteRuleStatusActive, actRes.Status)
	_, activated = actRes.ActivatedAt.Get()
	assert.True(t, activated)
	published, isLive := actRes.Live.Get()
	require.True(t, isLive)
	assert.EqualValues(t, 1, published.Number, "the draft became live")
	assert.True(t, actRes.Draft.Null)

	upd, err := c.SiteRulesUpdate(ctx, &siteapi.SiteUpdateRuleInput{
		Name:          siteapi.NewOptString("Tag the clickers"),
		TriggerEvents: []string{"email.clicked"},
	}, siteapi.SiteRulesUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	updRes, ok := upd.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", upd)
	assert.Equal(t, "Tag the clickers", updRes.Name)
	edited, drafted := updRes.Draft.Get()
	require.True(t, drafted, "an edit of a live Rule is a draft")
	assert.EqualValues(t, 2, edited.Number)
	assert.Equal(t, []string{"email.clicked"}, edited.TriggerEvents)
	assert.Len(t, edited.Actions, 2, "an update that leaves the actions out keeps them")
	stillLive, _ := updRes.Live.Get()
	assert.Equal(t, []string{"email.clicked", "link_followed"}, stillLive.TriggerEvents, "the live version is untouched")
	assert.Equal(t, siteapi.SiteRuleStatusActive, updRes.Status)

	deact, err := c.SiteRulesDeactivate(ctx, siteapi.SiteRulesDeactivateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	deactRes, ok := deact.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", deact)
	assert.Equal(t, siteapi.SiteRuleStatusInactive, deactRes.Status)

	got, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)
}

func TestSiteRulesListIsTheWorkspacesOwn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := c.SiteRulesList(context.Background(), siteapi.SiteRulesListParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})
	require.NoError(t, err)

	listed, ok := list.(*siteapi.SiteRulesListOK)
	require.Truef(t, ok, "got %T", list)
	ids := lo.Map(listed.Items, func(r siteapi.SiteRuleResource, _ int) int64 { return int64(r.ID) })
	assert.Contains(t, ids, int64(fixtures.RuleTagClickedID))
	assert.NotContains(t, ids, int64(fixtures.RuleGlobexID))
}

func TestSiteRulesAreRefusedWhenInvalid(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	unknownField := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:no_such_field","operator":"eq","value":"x"}]}`
	apply := []siteapi.SiteRuleAction{tagAction(siteapi.SiteRuleActionTypeApplyTag, "x")}

	unknown, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "Typo", TriggerEvents: []string{"a"}, Actions: apply,
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, unknownField)),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesCreateUnprocessableEntity{}, unknown, "a custom field the Workspace does not have is refused on save")

	noTag, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "No tag", TriggerEvents: []string{"a"},
		Actions: []siteapi.SiteRuleAction{{Type: siteapi.SiteRuleActionTypeApplyTag}},
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesCreateUnprocessableEntity{}, noTag)

	blankTrigger, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "Blank trigger", TriggerEvents: []string{"  "}, Actions: apply,
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesCreateUnprocessableEntity{}, blankTrigger)

	typoUpdate, err := c.SiteRulesUpdate(ctx, &siteapi.SiteUpdateRuleInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, unknownField)),
	}, siteapi.SiteRulesUpdateParams{WorkspaceId: workspaceID, ID: fixtures.RuleTagClickedID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesUpdateUnprocessableEntity{}, typoUpdate)
}

func TestSiteTriggerLeafIsAcceptedInARuleAndRefusedInASegment(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	leaf := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"trigger:url","operator":"contains","value":"pricing"}]}`

	rule, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "Pricing", TriggerEvents: []string{"a"},
		Actions:   []siteapi.SiteRuleAction{tagAction(siteapi.SiteRuleActionTypeApplyTag, "x")},
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, leaf)),
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRuleResource{}, rule)

	segment, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name: "Pricing", Condition: testhelper.SiteRuleGroup(t, leaf),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, segment)
}

func TestSiteRulesPublishRefusesARuleWithNoActions(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	created, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{Name: "Empty", TriggerEvents: []string{"a"}},
		siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created.(*siteapi.SiteRuleResource)

	out, err := c.SiteRulesPublish(ctx, siteapi.SiteRulesPublishParams{WorkspaceId: workspaceID, ID: res.ID})

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesPublishUnprocessableEntity{}, out)
}

func TestSiteRulesStayInTheirWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	get, err := c.SiteRulesGet(ctx, siteapi.SiteRulesGetParams{WorkspaceId: workspaceID, ID: fixtures.RuleGlobexID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesGetNotFound{}, get)

	published, err := c.SiteRulesPublish(ctx, siteapi.SiteRulesPublishParams{WorkspaceId: workspaceID, ID: fixtures.RuleGlobexID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesPublishNotFound{}, published)

	firings, err := c.SiteRuleFiringsList(ctx, siteapi.SiteRuleFiringsListParams{WorkspaceId: workspaceID, RuleId: fixtures.RuleGlobexID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRuleFiringsListNotFound{}, firings)
}

func TestSiteRuleFiringsShowWhatEachActionDid(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteRuleFiringsList(context.Background(), siteapi.SiteRuleFiringsListParams{
		WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), RuleId: fixtures.RuleTagClickedID,
	})

	require.NoError(t, err)
	listed, ok := out.(*siteapi.SiteRuleFiringsListOK)
	require.Truef(t, ok, "got %T", out)
	require.Len(t, listed.Items, 2)
	pending, done := listed.Items[0], listed.Items[1] // newest first
	assert.EqualValues(t, fixtures.RuleFiringPendingID, pending.ID)
	assert.Empty(t, pending.Results)
	_, finished := pending.CompletedAt.Get()
	assert.False(t, finished)
	assert.EqualValues(t, fixtures.RuleFiringDoneID, done.ID)
	assert.EqualValues(t, fixtures.ContactAliceID, done.ContactId)
	assert.Equal(t, "rule_click", done.Action)
	require.Len(t, done.Results, 1)
	assert.Equal(t, siteapi.SiteRuleActionStatusSucceeded, done.Results[0].Status)
}
