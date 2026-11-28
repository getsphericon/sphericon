package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// httpSecret is the signing secret the HTTP steps and http Actions of these tests are saved with.
var httpSecret = testhelper.SigningSecret("http")

func TestSiteHTTPStepIsMaskedOnReadKeptOnEditAndAbsentFromTheAuditLog(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	body := `{"text":"{{ first_name }}"}`

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Notify Slack", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(siteapi.HttpStepInput{
			Name:    "notify",
			Body:    body,
			URL:     siteapi.NewOptString("https://hooks.slack.com/services/T0/SECRETPATH"),
			Headers: []siteapi.AutomationHttpHeaderInput{{Name: "Authorization", Value: siteapi.NewOptString("Bearer hunter2")}},
			Secret:  siteapi.NewOptSigningSecret(siteapi.SigningSecret(httpSecret)),
		}),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created

	read := res.Draft.Value.Steps.Steps[0]
	require.Equal(t, siteapi.HttpStepAutomationStepSum, read.OneOf.Type)
	step := read.OneOf.HttpStep
	sealedAtCreate := sealedValues(t, env, int64(res.ID))
	assert.NotEmpty(t, step.ID)
	assert.True(t, step.HasSecret)
	raw, err := json.Marshal(read)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"url"`, "the url is never returned")
	assert.NotContains(t, string(raw), `"secret"`)
	require.Len(t, step.Headers, 1)
	assert.Equal(t, "Authorization", step.Headers[0].Name)
	assert.False(t, step.Headers[0].Value.Set, "header values are never returned")

	// Editing the body and resending the step id and the header names keeps the stored secrets.
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))),
		Steps: testhelper.SitePathEdit(siteapi.HttpStepInput{
			Name: "notify",
			ID:   siteapi.NewOptStepId(step.ID), Body: `{"text":"edited"}`,
			Headers: []siteapi.AutomationHttpHeaderInput{{Name: step.Headers[0].Name}},
		})},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.True(t, updated.Draft.Value.Steps.Steps[0].OneOf.HttpStep.HasSecret)

	steps := testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID))
	call := storedHTTP(t, steps, 0)
	stored := httpcall.Spec{URL: call.URL, Headers: call.Headers, Secret: call.Secret}
	opened, err := httpcall.Open(env.Cipher, stored)
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T0/SECRETPATH", opened.URL)
	assert.Equal(t, "Bearer hunter2", opened.Headers["Authorization"])
	assert.Equal(t, httpSecret, opened.Secret)

	entries := env.QueuedEvents(t, events.NameAuditEntry)
	require.NotEmpty(t, entries)
	for _, e := range entries {
		raw, err := json.Marshal(e)
		require.NoError(t, err)
		secrets := append([]string{"SECRETPATH", "hunter2", httpSecret}, sealedAtCreate...)
		secrets = append(secrets, call.URL, call.Secret, call.Headers[0].Value)
		for _, secret := range secrets {
			assert.NotContains(t, string(raw), secret)
		}
	}
}

// sealedValues returns the ciphertext of the first step's URL, secret and header values.
func sealedValues(t *testing.T, env *testhelper.TestEnv, id int64) []string {
	t.Helper()
	call := storedHTTP(t, testhelper.LatestSteps(t.Context(), t, env.DB, id), 0)
	out := make([]string, 0, 2+len(call.Headers))
	out = append(out, call.URL, call.Secret)
	for _, h := range call.Headers {
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
	ctx := t.Context()
	params := siteapi.SiteAutomationsDryRunHttpStepParams{WorkspaceId: fixtures.AcmePublicID}
	contact := siteapi.EntityId(fixtures.ContactAliceID)

	ok, err := c.SiteAutomationsDryRunHttpStep(ctx, &siteapi.DryRunHttpStepInput{Body: `{"name":"{{ first_name }}"}`, ContactId: contact}, params)
	require.NoError(t, err)
	rendered := ok
	assert.JSONEq(t, `{"name":"Alice"}`, rendered.Body.Or(""))

	bad, err := c.SiteAutomationsDryRunHttpStep(ctx, &siteapi.DryRunHttpStepInput{Body: `{"n": {{ nope }}}`, ContactId: contact}, params)
	require.NoError(t, err)
	failed := bad
	assert.Equal(t, siteapi.AutomationHttpErrorKindInvalidBody, failed.ErrorKind.Or(""))
	assert.False(t, failed.Body.Set)
}

func TestSiteHTTPStepRefusedAtSaveIsUnprocessable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
		Name: "Bad", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(siteapi.HttpStepInput{
			Name: "notify",
			URL:  siteapi.NewOptString("https://hooks.example.com/x"),
			Body: `{"a": {{ nope }}}`,
		}),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestSiteAutomationRefusesInfrastructureEventsAsTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	params := siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID}

	for _, trigger := range []string{systemevent.EmailDelivered, systemevent.EmailDelayed, systemevent.EmailDropped, systemevent.EmailBounced, systemevent.EmailComplained, systemevent.EmailSent} {
		_, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: siteapi.Name("From " + trigger), Trigger: testhelper.SiteTrigger(trigger)}, params)
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, trigger)
	}

	for _, trigger := range []string{systemevent.EmailOpened, systemevent.EmailClicked, systemevent.EmailUnsubscribed} {
		_, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: siteapi.Name("From " + trigger), Trigger: testhelper.SiteTrigger(trigger)}, params)
		require.NoError(t, err, trigger)
	}

	_, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(idStr(fixtures.AutomationWinBackID)))), Trigger: siteapi.NewOptAutomationTriggerInput(testhelper.SiteTrigger(systemevent.EmailBounced))},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AutomationWinBackID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}
