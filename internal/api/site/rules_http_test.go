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

func TestSiteRuleHTTPActionIsMaskedOnReadAndKeptOnEdit(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{
		Name: "Call Slack", TriggerEvents: []string{"signup"},
		Actions: []siteapi.SiteRuleAction{{
			Type: siteapi.SiteRuleActionTypeHTTP, Body: siteapi.NewOptString(`{"text":"{{ first_name }}"}`),
			URL:     siteapi.NewOptString("https://hooks.slack.com/services/T0/SECRETPATH"),
			Headers: []siteapi.SiteAutomationHttpHeader{{Name: "Authorization", Value: siteapi.NewOptString("Bearer hunter2")}},
			Secret:  siteapi.NewOptString("whsec_topsecret"),
		}},
	}, siteapi.SiteRulesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", created)

	action := res.Actions[0]
	assert.NotEmpty(t, action.ID.Or(""))
	assert.True(t, action.HasSecret.Or(false))
	assert.False(t, action.URL.Set, "the url is never returned")
	assert.False(t, action.Secret.Set)
	require.Len(t, action.Headers, 1)
	assert.Equal(t, "Authorization", action.Headers[0].Name)
	assert.False(t, action.Headers[0].Value.Set, "header values are never returned")

	// Resending only the action id keeps the stored secrets.
	action.Body = siteapi.NewOptString(`{"text":"edited"}`)
	updated, err := c.SiteRulesUpdate(ctx, &siteapi.SiteUpdateRuleInput{Actions: []siteapi.SiteRuleAction{action}},
		siteapi.SiteRulesUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteRuleResource)
	require.Truef(t, ok, "got %T", updated)
	assert.True(t, upd.Actions[0].HasSecret.Or(false))
	assert.Equal(t, action.ID, upd.Actions[0].ID)
}
