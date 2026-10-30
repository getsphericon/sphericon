package site_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteHTTPStepIsMaskedOnReadKeptOnEditAndAbsentFromTheAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	body := `{"text":"{{ first_name }}"}`

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Notify Slack", TriggerEvent: "signup",
		Steps: []siteapi.SiteAutomationStep{{
			Type: siteapi.SiteAutomationStepTypeHTTP, Body: siteapi.NewOptString(body),
			URL:     siteapi.NewOptString("https://hooks.slack.com/services/T0/SECRETPATH"),
			Headers: []siteapi.SiteAutomationHttpHeader{{Name: "Authorization", Value: siteapi.NewOptString("Bearer hunter2")}},
			Secret:  siteapi.NewOptString("whsec_topsecret"),
		}},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", created)

	step := res.Draft.Value.Steps[0]
	sealedAtCreate := sealedValues(t, env, int64(res.ID))
	assert.NotEmpty(t, step.ID.Or(""))
	assert.True(t, step.HasSecret.Or(false))
	assert.False(t, step.URL.Set, "the url is never returned")
	assert.False(t, step.Secret.Set)
	require.Len(t, step.Headers, 1)
	assert.Equal(t, "Authorization", step.Headers[0].Name)
	assert.False(t, step.Headers[0].Value.Set, "header values are never returned")

	// Editing the body and resending only the step id keeps the stored secrets.
	step.Body = siteapi.NewOptString(`{"text":"edited"}`)
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{Steps: []siteapi.SiteAutomationStep{step}},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", updated)
	assert.True(t, upd.Draft.Value.Steps[0].HasSecret.Or(false))

	steps, err := automations.Decode(testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID)))
	require.NoError(t, err)
	opened, err := automations.OpenHTTP(env.Cipher, steps[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T0/SECRETPATH", opened.URL)
	assert.Equal(t, "Bearer hunter2", opened.Headers["Authorization"])
	assert.Equal(t, "whsec_topsecret", opened.Secret)

	entries := env.OutboxEvents(t, events.NameAuditEntry)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		secrets := append([]string{"SECRETPATH", "hunter2", "whsec_topsecret"}, sealedAtCreate...)
		secrets = append(secrets, steps[0].URL, steps[0].Secret, steps[0].Headers[0].Value)
		for _, secret := range secrets {
			assert.NotContains(t, string(raw), secret)
		}
	}
}

// sealedValues returns the ciphertext of the first step's URL, secret and header values.
func sealedValues(t *testing.T, env *testhelper.TestEnv, id int64) []string {
	t.Helper()
	steps, err := automations.Decode(testhelper.LatestSteps(context.Background(), t, env.DB, id))
	require.NoError(t, err)
	out := make([]string, 0, 2+len(steps[0].Headers))
	out = append(out, steps[0].URL, steps[0].Secret)
	for _, h := range steps[0].Headers {
		out = append(out, h.Value)
	}
	for _, v := range out {
		require.NotEmpty(t, v)
	}
	return out
}

func TestSiteHTTPStepDryRunRendersForAContactWithoutCalling(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteAutomationsDryRunHttpStepParams{WorkspaceId: fixtures.AcmePublicID}
	contact := siteapi.EntityId(fixtures.ContactAliceID)

	ok, err := c.SiteAutomationsDryRunHttpStep(ctx, &siteapi.SiteDryRunHttpStepInput{Body: `{"name":"{{ first_name }}"}`, ContactId: contact}, params)
	require.NoError(t, err)
	rendered, isResult := ok.(*siteapi.SiteDryRunHttpStepResult)
	require.Truef(t, isResult, "got %T", ok)
	assert.JSONEq(t, `{"name":"Alice"}`, rendered.Body.Or(""))

	bad, err := c.SiteAutomationsDryRunHttpStep(ctx, &siteapi.SiteDryRunHttpStepInput{Body: `{"n": {{ nope }}}`, ContactId: contact}, params)
	require.NoError(t, err)
	failed, isResult := bad.(*siteapi.SiteDryRunHttpStepResult)
	require.Truef(t, isResult, "got %T", bad)
	assert.Equal(t, siteapi.SiteAutomationHttpErrorKindInvalidBody, failed.ErrorKind.Or(""))
	assert.False(t, failed.Body.Set)
}

func TestSiteHTTPStepRefusedAtSaveIsUnprocessable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := c.SiteAutomationsCreate(context.Background(), &siteapi.SiteCreateAutomationInput{
		Name: "Bad", TriggerEvent: "signup",
		Steps: []siteapi.SiteAutomationStep{{
			Type: siteapi.SiteAutomationStepTypeHTTP, URL: siteapi.NewOptString("https://hooks.example.com/x"),
			Body: siteapi.NewOptString(`{"a": {{ nope }}}`),
		}},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteAutomationsCreateUnprocessableEntity), res)
}

func TestSiteAutomationRefusesInfrastructureEventsAsTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	for _, trigger := range []string{"email.delivered", "email.delayed", "email.dropped", "email.bounced", "email.complained", "email.sent"} {
		res, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "From " + trigger, TriggerEvent: trigger}, params)
		require.NoError(t, err)
		assert.IsType(t, new(siteapi.SiteAutomationsCreateUnprocessableEntity), res, trigger)
	}

	for _, trigger := range []string{"email.opened", "email.clicked", "email.unsubscribed"} {
		res, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "From " + trigger, TriggerEvent: trigger}, params)
		require.NoError(t, err)
		assert.IsType(t, new(siteapi.SiteAutomationResource), res, trigger)
	}

	made, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "Plain", TriggerEvent: "signup"}, params)
	require.NoError(t, err)
	created, ok := made.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", made)
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{TriggerEvent: siteapi.NewOptString("email.bounced")},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, new(siteapi.SiteAutomationsUpdateUnprocessableEntity), updated)
}
