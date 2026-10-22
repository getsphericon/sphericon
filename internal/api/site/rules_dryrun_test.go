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

func TestSiteRulesDryRunShowsTheActionsAndWritesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	firings := env.DB.RuleFiring.Query().CountX(ctx)
	tags := env.DB.ContactTag.Query().CountX(ctx)
	params := siteapi.SiteRulesDryRunParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: fixtures.RuleTagClickedID}

	out, err := c.SiteRulesDryRun(ctx, &siteapi.SiteDryRunRuleInput{EventId: fixtures.EventRuleClickLiamID}, params)

	require.NoError(t, err)
	res, ok := out.(*siteapi.SiteRuleDryRunResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.SiteRuleDryRunOutcomeWouldFire, res.Outcome)
	require.Len(t, res.Actions, 1)
	assert.Equal(t, "apply_tag", res.Actions[0].Type)
	assert.Contains(t, res.Actions[0].Summary, "rule-clicked")
	assert.Equal(t, firings, env.DB.RuleFiring.Query().CountX(ctx), "no Firing")
	assert.Equal(t, tags, env.DB.ContactTag.Query().CountX(ctx), "no Tag change")
}

func TestSiteRulesDryRunSaysWhyItWouldNotFire(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteRulesDryRunParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: fixtures.RuleVipOnlyID}

	out, err := c.SiteRulesDryRun(ctx, &siteapi.SiteDryRunRuleInput{EventId: fixtures.EventRuleVipLiamID}, params)

	require.NoError(t, err)
	res, ok := out.(*siteapi.SiteRuleDryRunResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.SiteRuleDryRunOutcomeConditionNotMet, res.Outcome)
	assert.Empty(t, res.Actions)
}

func TestSiteRulesDryRunRefusesAnotherWorkspacesEvent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	params := siteapi.SiteRulesDryRunParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: fixtures.RuleTagClickedID}

	out, err := c.SiteRulesDryRun(context.Background(), &siteapi.SiteDryRunRuleInput{EventId: fixtures.EventGlobexID}, params)

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteRulesDryRunNotFound{}, out)
}
