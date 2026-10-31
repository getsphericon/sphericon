package automations_test

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const slackBody = `{"text": "New signup: {{ first_name }} {{ last_name }}", "email": "{{ email }}"}`

func httpStep(url string) automations.Step {
	return automations.Step{Type: automations.StepHTTP, URL: url, Body: slackBody}
}

func TestDryRunEscapesAHostileContactNameAndKeepsTheStructure(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := acme.Contact().UpdateOneID(fixtures.ContactAliceID).
		SetFirstName(`Al"ice`).SetLastName(`\","admin":true,"x":"`).Save(ctx)
	require.NoError(t, err)

	res, err := m.DryRun(ctx, acme, fixtures.ContactAliceID, slackBody)
	require.NoError(t, err)
	require.Empty(t, res.ErrorKind)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Body), &got))
	assert.Equal(t, []string{"email", "text"}, slices.Sorted(maps.Keys(got)), "the structure is the template's")
	assert.Equal(t, `New signup: Al"ice \","admin":true,"x":"`, got["text"])
}

func TestDryRunNamesTheErrorKindOfAnInvalidBody(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	acme := env.DB.Scoped(fixtures.AcmeID)

	res, err := m.DryRun(context.Background(), acme, fixtures.ContactAliceID, `{"n": {{ missing }}}`)
	require.NoError(t, err)
	assert.Equal(t, automations.ErrorKindInvalidBody, res.ErrorKind)
	assert.Empty(t, res.Body)

	res, err = m.DryRun(context.Background(), acme, fixtures.ContactAliceID, `{"a": {% if %}}`)
	require.NoError(t, err)
	assert.Equal(t, automations.ErrorKindInvalidBody, res.ErrorKind)
}

func TestDryRunOfAContactOfAnotherWorkspaceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.DryRun(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceID, slackBody)
	assert.ErrorIs(t, err, automations.ErrContactNotFound)
}

func TestSavingRefusesAnHTTPStepThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	headers := func(n int) []automations.Header {
		out := make([]automations.Header, n)
		for i := range out {
			out[i] = automations.Header{Name: "X-H" + strings.Repeat("a", i), Value: "v"}
		}
		return out
	}
	invalidJSON := httpStep("https://hooks.example.com/x")
	invalidJSON.Body = `{"a": {{ nope }}}`
	longURL := httpStep("https://hooks.example.com/" + strings.Repeat("a", 2048))
	bigBody := httpStep("https://hooks.example.com/x")
	bigBody.Body = `{"a":"` + strings.Repeat("a", 64<<10) + `"}`
	manyHeaders := httpStep("https://hooks.example.com/x")
	manyHeaders.Headers = headers(6)
	notHTTP := httpStep("ftp://hooks.example.com/x")
	noURL := httpStep("")
	tooMany := make([]automations.Step, 11)
	for i := range tooMany {
		tooMany[i] = httpStep("https://hooks.example.com/x")
	}

	for name, steps := range map[string][]automations.Step{
		"invalid json":      {invalidJSON},
		"url over 2 KiB":    {longURL},
		"body over 64 KiB":  {bigBody},
		"six headers":       {manyHeaders},
		"not an http url":   {notHTTP},
		"a new step no url": {noURL},
		"eleven steps":      tooMany,
	} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, TriggerEvent: "signup", Steps: steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)
	}

	five := httpStep("https://hooks.example.com/x")
	five.Headers = headers(5)
	ten := make([]automations.Step, 10)
	for i := range ten {
		ten[i] = httpStep("https://hooks.example.com/x")
	}
	for name, steps := range map[string][]automations.Step{"five headers": {five}, "ten steps": ten} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, TriggerEvent: "signup", Steps: steps})
		assert.NoError(t, err, name)
	}
}

func TestFixtureHTTPStepOpensAndItsBodyRenders(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	acme := env.DB.Scoped(fixtures.AcmeID)

	a, err := m.Get(context.Background(), acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	steps := liveSteps(t, a)
	opened, err := automations.OpenHTTP(env.Cipher, steps[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/B000/XXXX", opened.URL)
	assert.Equal(t, "Bearer slack-demo-token", opened.Headers["Authorization"])

	res, err := m.DryRun(context.Background(), acme, fixtures.ContactAliceID, steps[0].Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text": "New signup: Alice Smith"}`, res.Body)
}

func TestStoredHTTPStepHoldsNoPlaintextAndAnEditKeepsTheSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	step := httpStep("https://hooks.slack.com/services/T000/SECRETPATH")
	step.Headers = []automations.Header{{Name: "Authorization", Value: "Bearer hunter2"}}
	step.Secret = "whsec_topsecret"
	created, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Notify", TriggerEvent: "signup", Steps: []automations.Step{{Type: automations.StepWait, Seconds: 1}, step},
	})
	require.NoError(t, err)
	for _, secret := range []string{"SECRETPATH", "hunter2", "whsec_topsecret"} {
		assert.NotContains(t, created.Edges.DraftVersion.Steps, secret)
	}

	stored := draftSteps(t, created)
	require.NotEmpty(t, stored[1].ID, "a new HTTP step gets an id")

	// An edit that resends only the id, with a new body, keeps URL, headers and secret.
	edit := automations.Step{
		Type: automations.StepHTTP, ID: stored[1].ID, Body: `{"edited":true}`,
		Headers: []automations.Header{{Name: "Authorization"}},
	}
	updated, err := m.Update(ctx, acme, created.ID, automations.UpdateInput{Steps: &[]automations.Step{edit}})
	require.NoError(t, err)
	after := draftSteps(t, updated)
	assert.Equal(t, stored[1].URL, after[0].URL, "the stored ciphertext is copied, not re-sealed")
	assert.Equal(t, stored[1].Secret, after[0].Secret)
	assert.Equal(t, stored[1].Headers, after[0].Headers)
	assert.Equal(t, `{"edited":true}`, after[0].Body)

	opened, err := automations.OpenHTTP(env.Cipher, after[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/SECRETPATH", opened.URL)
	assert.Equal(t, map[string]string{"Authorization": "Bearer hunter2"}, opened.Headers)
	assert.Equal(t, "whsec_topsecret", opened.Secret)

	// A new URL, header and a cleared secret replace what was stored.
	replace := automations.Step{
		Type: automations.StepHTTP, ID: stored[1].ID, URL: "https://hooks.zapier.com/a", Body: slackBody,
		Headers: []automations.Header{{Name: "X-Key", Value: "k"}}, ClearSecret: true,
	}
	updated, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Steps: &[]automations.Step{replace}})
	require.NoError(t, err)
	after = draftSteps(t, updated)
	opened, err = automations.OpenHTTP(env.Cipher, after[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.zapier.com/a", opened.URL)
	assert.Equal(t, map[string]string{"X-Key": "k"}, opened.Headers)
	assert.Empty(t, opened.Secret)

	// A header with no stored value to keep is refused.
	bad := automations.Step{Type: automations.StepHTTP, ID: stored[1].ID, Body: slackBody, Headers: []automations.Header{{Name: "Nope"}}}
	_, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Steps: &[]automations.Step{bad}})
	assert.ErrorIs(t, err, automations.ErrInvalidStep)
}
