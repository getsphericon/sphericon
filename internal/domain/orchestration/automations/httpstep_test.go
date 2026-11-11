package automations_test

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const slackBody = `{"text": "New signup: {{ first_name }} {{ last_name }}", "email": "{{ email }}"}`

// openStored decrypts a stored HTTP step's URL, header values and secret, as its call opens them.
func openStored(cipher *secrets.Cipher, s automationstep.Step) (httpcall.Opened, error) {
	return httpcall.Open(cipher, httpcall.Spec{URL: s.URL, Headers: s.Headers, Secret: s.Secret})
}

func httpStep(url string) automationstep.Step {
	return automationstep.Step{Type: automationstep.HTTP, URL: url, Body: slackBody}
}

// Fixtures: Babel's Contact with quotes in both names, the last one an attempt to add a key.
func TestDryRunEscapesAHostileContactNameAndKeepsTheStructure(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()

	res, err := m.DryRun(ctx, env.DB.Scoped(fixtures.BabelID), fixtures.ContactHostileQuotesID, slackBody)
	require.NoError(t, err)
	require.Empty(t, res.ErrorKind)

	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(res.Body), &got))
	assert.Equal(t, []string{"email", "text"}, slices.Sorted(maps.Keys(got)), "the structure is the template's")
	assert.Equal(t, `New signup: Al"ice \","admin":true,"x":"`, got["text"])
}

func TestDryRunNamesTheErrorKindOfAnInvalidBody(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
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
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	_, err := m.DryRun(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceID, slackBody)
	assert.ErrorIs(t, err, automations.ErrContactNotFound)
}

func TestSavingRefusesAnHTTPStepThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	headers := func(n int) []httpcall.Header {
		out := make([]httpcall.Header, n)
		for i := range out {
			out[i] = httpcall.Header{Name: "X-H" + strings.Repeat("a", i), Value: "v"}
		}
		return out
	}
	invalidJSON := httpStep("https://hooks.example.com/x")
	invalidJSON.Body = `{"a": {{ nope }}}`
	longURL := httpStep("https://hooks.example.com/" + strings.Repeat("a", 2048))
	bigBody := httpStep("https://hooks.example.com/x")
	bigBody.Body = `{"a":"` + strings.Repeat("a", 64<<10) + `"}`
	notHTTP := httpStep("ftp://hooks.example.com/x")
	noURL := httpStep("")
	tooMany := make([]automationstep.Step, 11)
	for i := range tooMany {
		tooMany[i] = httpStep("https://hooks.example.com/x")
	}

	for name, steps := range map[string][]automationstep.Step{
		"invalid json":      {invalidJSON},
		"url over 2 KiB":    {longURL},
		"body over 64 KiB":  {bigBody},
		"not an http url":   {notHTTP},
		"a new step no url": {noURL},
		"eleven steps":      tooMany,
	} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, TriggerEvent: "signup", Steps: steps})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)
	}

	five := httpStep("https://hooks.example.com/x")
	five.Headers = headers(5)
	ten := make([]automationstep.Step, 10)
	for i := range ten {
		ten[i] = httpStep("https://hooks.example.com/x")
	}
	for name, steps := range map[string][]automationstep.Step{"five headers": {five}, "ten steps": ten} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, TriggerEvent: "signup", Steps: steps})
		assert.NoError(t, err, name)
	}
}

func TestFixtureHTTPStepOpensAndItsBodyRenders(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)

	a, err := m.Get(context.Background(), acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	steps := liveSteps(t, a)
	opened, err := openStored(env.Cipher, steps[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/B000/XXXX", opened.URL)
	assert.Equal(t, "Bearer slack-demo-token", opened.Headers["Authorization"])

	res, err := m.DryRun(context.Background(), acme, fixtures.ContactAliceID, steps[0].Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text": "New signup: Alice Smith"}`, res.Body)
}

func TestStoredHTTPStepHoldsNoPlaintextAndAnEditKeepsTheSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := context.Background()

	step := httpStep("https://hooks.slack.com/services/T000/SECRETPATH")
	step.Headers = []httpcall.Header{{Name: "Authorization", Value: "Bearer hunter2"}}
	step.Secret = "whsec_topsecret"
	created, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Notify", TriggerEvent: "signup", Steps: []automationstep.Step{{Type: automationstep.Wait, Seconds: 1}, step},
	})
	require.NoError(t, err)
	for _, secret := range []string{"SECRETPATH", "hunter2", "whsec_topsecret"} {
		assert.NotContains(t, created.Edges.DraftVersion.Steps, secret)
	}

	stored := draftSteps(t, created)
	require.NotEmpty(t, stored[1].ID, "a new HTTP step gets an id")

	// An edit that resends only the id, with a new body, keeps URL, headers and secret.
	edit := automationstep.Step{
		Type: automationstep.HTTP, ID: stored[1].ID, Body: `{"edited":true}`,
		Headers: []httpcall.Header{{Name: "Authorization"}},
	}
	updated, err := m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: &[]automationstep.Step{edit}})
	require.NoError(t, err)
	after := draftSteps(t, updated)
	assert.Equal(t, stored[1].URL, after[0].URL, "the stored ciphertext is copied, not re-sealed")
	assert.Equal(t, stored[1].Secret, after[0].Secret)
	assert.Equal(t, stored[1].Headers, after[0].Headers)
	assert.Equal(t, `{"edited":true}`, after[0].Body)

	opened, err := openStored(env.Cipher, after[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/SECRETPATH", opened.URL)
	assert.Equal(t, map[string]string{"Authorization": "Bearer hunter2"}, opened.Headers)
	assert.Equal(t, "whsec_topsecret", opened.Secret)

	// A new URL, header and a cleared secret replace what was stored.
	replace := automationstep.Step{
		Type: automationstep.HTTP, ID: stored[1].ID, URL: "https://hooks.zapier.com/a", Body: slackBody,
		Headers: []httpcall.Header{{Name: "X-Key", Value: "k"}}, ClearSecret: true,
	}
	updated, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: &[]automationstep.Step{replace}})
	require.NoError(t, err)
	after = draftSteps(t, updated)
	opened, err = openStored(env.Cipher, after[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.zapier.com/a", opened.URL)
	assert.Equal(t, map[string]string{"X-Key": "k"}, opened.Headers)
	assert.Empty(t, opened.Secret)

	// A header with no stored value to keep is refused.
	bad := automationstep.Step{Type: automationstep.HTTP, ID: stored[1].ID, Body: slackBody, Headers: []httpcall.Header{{Name: "Nope"}}}
	_, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: &[]automationstep.Step{bad}})
	assert.ErrorIs(t, err, automations.ErrInvalidStep)
}
