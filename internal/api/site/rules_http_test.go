package site_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	ht "github.com/ogen-go/ogen/http"

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

	action := res.Draft.Value.Actions[0]
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
	assert.True(t, upd.Draft.Value.Actions[0].HasSecret.Or(false))
	assert.Equal(t, action.ID, upd.Draft.Value.Actions[0].ID)
}

// The contract holds an http Action to five request headers, so a sixth is refused before the
// Rule is read.
func TestSiteRuleHTTPActionRefusesASixthHeader(t *testing.T) {
	env := testhelper.Setup(t)
	headers := make([]string, 6)
	for i := range headers {
		headers[i] = fmt.Sprintf(`{"name":"X-H%d","value":"v"}`, i)
	}
	body := `{"name":"Call","triggerEvents":["signup"],"actions":[{"type":"http","url":"https://hooks.example.com/x",` +
		`"body":"{}","headers":[` + strings.Join(headers, ",") + `]}]}`
	raw := env.SiteActorVia(t, fixtures.OwnerJohnEmail, func(inner ht.Client) ht.Client {
		return rawBody{inner: inner, body: body}
	})

	out, err := raw.SiteRulesCreate(context.Background(), &siteapi.SiteCreateRuleInput{},
		siteapi.SiteRulesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	bad, ok := out.(*siteapi.SiteRulesCreateBadRequest)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusBadRequest), bad.Status)
}
