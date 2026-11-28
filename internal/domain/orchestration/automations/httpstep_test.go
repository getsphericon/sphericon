package automations_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// stepSecret is the signing secret the HTTP steps of these tests are saved with.
var stepSecret = testhelper.SigningSecret("step")

const slackBody = `{"text": "New signup: {{ first_name }} {{ last_name }}", "email": "{{ email }}"}`

// openStored decrypts a stored HTTP step's URL, header values and secret, as its call opens them.
func openStored(cipher *secrets.Cipher, p automationstep.Payload) (httpcall.Opened, error) {
	s := p.(automationstep.HTTP)
	return httpcall.Open(cipher, httpcall.Spec{URL: s.URL, Headers: s.Headers, Secret: s.Secret})
}

func httpStep(url string) automationstep.HTTP {
	return automationstep.HTTP{Name: "notify", URL: url, Body: slackBody}
}

// Fixtures: Babel's Contact with quotes in both names, the last one an attempt to add a key.
func TestDryRunEscapesAHostileContactNameAndKeepsTheStructure(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := t.Context()

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
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)

	res, err := m.DryRun(t.Context(), acme, fixtures.ContactAliceID, `{"n": {{ missing }}}`)
	require.NoError(t, err)
	assert.Equal(t, automations.ErrorKindInvalidBody, res.ErrorKind)
	assert.Empty(t, res.Body)

	res, err = m.DryRun(t.Context(), acme, fixtures.ContactAliceID, `{"a": {% if %}}`)
	require.NoError(t, err)
	assert.Equal(t, automations.ErrorKindInvalidBody, res.ErrorKind)
}

func TestDryRunOfAContactOfAnotherWorkspaceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	_, err := m.DryRun(t.Context(), env.DB.Scoped(fixtures.GlobexID), fixtures.ContactAliceID, slackBody)
	assert.ErrorIs(t, err, automations.ErrContactNotFound)
}

func TestSavingRefusesAnHTTPStepThatCannotBeSent(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()

	headers := func(n int) []httpheader.Header {
		out := make([]httpheader.Header, n)
		for i := range out {
			out[i] = httpheader.Header{Name: "X-H" + strings.Repeat("a", i), Value: "v"}
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
	tooMany := make([]automationstep.Payload, 11)
	for i := range tooMany {
		call := httpStep("https://hooks.example.com/x")
		call.Name = fmt.Sprintf("notify_%d", i)
		tooMany[i] = call
	}
	six := httpStep("https://hooks.example.com/x")
	six.Headers = headers(6)
	emptySecret := httpStep("https://hooks.example.com/x")
	emptySecret.Secret = "whsec_"
	notBase64 := httpStep("https://hooks.example.com/x")
	notBase64.Secret = "whsec_not base64"

	for name, refused := range map[string][]automationstep.Payload{
		"invalid json":      {invalidJSON},
		"url over 2 KiB":    {longURL},
		"body over 64 KiB":  {bigBody},
		"not an http url":   {notHTTP},
		"a new step no url": {noURL},
		"eleven steps":      tooMany,
		"six headers":       {six},
		"an empty secret":   {emptySecret},
		"a secret no key":   {notBase64},
	} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signup"}}, Steps: steps(refused...)})
		require.ErrorIs(t, err, automations.ErrInvalidStep, name)
	}

	five := httpStep("https://hooks.example.com/x")
	five.Headers = headers(5)
	ten := make([]automationstep.Payload, 10)
	for i := range ten {
		call := httpStep("https://hooks.example.com/x")
		call.Name = fmt.Sprintf("notify_%d", i)
		ten[i] = call
	}
	for name, kept := range map[string][]automationstep.Payload{"five headers": {five}, "ten steps": ten} {
		_, err := m.Create(ctx, acme, automations.CreateInput{Name: name, Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signup"}}, Steps: steps(kept...)})
		assert.NoError(t, err, name)
	}
}

func TestFixtureHTTPStepOpensAndItsBodyRenders(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)

	a, err := m.Get(t.Context(), acme, fixtures.AutomationNotifySlackID)
	require.NoError(t, err)
	steps := liveSteps(t, a)
	opened, err := openStored(env.Cipher, steps[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/B000/XXXX", opened.URL)
	assert.Equal(t, "Bearer slack-demo-token", opened.Headers["Authorization"])

	res, err := m.DryRun(t.Context(), acme, fixtures.ContactAliceID, steps[0].(automationstep.HTTP).Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text": "New signup: Alice Smith"}`, res.Body)
}

func TestStoredHTTPStepHoldsNoPlaintextAndAnEditKeepsTheSecrets(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()

	step := httpStep("https://hooks.slack.com/services/T000/SECRETPATH")
	step.Headers = []httpheader.Header{{Name: "Authorization", Value: "Bearer hunter2"}}
	step.Secret = stepSecret
	created, err := m.Create(ctx, acme, automations.CreateInput{
		Name: "Notify", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signup"}}, Steps: steps(automationstep.Wait{Seconds: 1}, step),
	})
	require.NoError(t, err)
	for _, secret := range []string{"SECRETPATH", "hunter2", stepSecret} {
		assert.NotContains(t, testhelper.Document(t, created.Edges.DraftVersion.Steps), secret)
	}

	_, draft := automations.LiveAndDraft(created)
	id := pathOf(t, draft).Steps[1].ID
	stored := draftSteps(t, created)[1].(automationstep.HTTP)
	require.NotEmpty(t, id, "a new HTTP step gets an id")

	// An edit that resends only the id, with a new body, keeps URL, headers and secret.
	edit := automationstep.Step{ID: id, Payload: automationstep.HTTP{
		Name: "notify",
		Body: `{"edited":true}`, Headers: []httpheader.Header{{Name: "Authorization"}},
	}}
	updated, err := m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: new(automationstep.Root(edit))})
	require.NoError(t, err)
	after := draftSteps(t, updated)[0].(automationstep.HTTP)
	assert.Equal(t, stored.URL, after.URL, "the stored ciphertext is copied, not re-sealed")
	assert.Equal(t, stored.Secret, after.Secret)
	assert.Equal(t, stored.Headers, after.Headers)
	assert.Equal(t, `{"edited":true}`, after.Body)

	opened, err := openStored(env.Cipher, after)
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.slack.com/services/T000/SECRETPATH", opened.URL)
	assert.Equal(t, map[string]string{"Authorization": "Bearer hunter2"}, opened.Headers)
	assert.Equal(t, stepSecret, opened.Secret)

	// A new URL, header and a cleared secret replace what was stored.
	replace := automationstep.Step{ID: id, Payload: automationstep.HTTP{
		Name: "notify",
		URL:  "https://hooks.zapier.com/a", Body: slackBody, Headers: []httpheader.Header{{Name: "X-Key", Value: "k"}}, ClearSecret: true,
	}}
	updated, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: new(automationstep.Root(replace))})
	require.NoError(t, err)
	opened, err = openStored(env.Cipher, draftSteps(t, updated)[0])
	require.NoError(t, err)
	assert.Equal(t, "https://hooks.zapier.com/a", opened.URL)
	assert.Equal(t, map[string]string{"X-Key": "k"}, opened.Headers)
	assert.Empty(t, opened.Secret)

	// A header with no stored value to keep is refused.
	bad := automationstep.Step{ID: id, Payload: automationstep.HTTP{Name: "notify", Body: slackBody, Headers: []httpheader.Header{{Name: "Nope"}}}}
	_, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: new(automationstep.Root(bad))})
	assert.ErrorIs(t, err, automations.ErrInvalidStep)
}

// An HTTP step sent with an id the stored steps do not hold is a new step: it keeps no secret of
// another step, so a new step without a URL is refused (ADR 0122: secrets are matched by Step id).
func TestAnHTTPStepWithAnUnknownIDKeepsNoStoredSecret(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)
	ctx := t.Context()
	created, err := m.Create(ctx, acme, automations.CreateInput{Name: "Notify", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "signup"}}, Steps: steps(httpStep("https://hooks.example.com/x"))})
	require.NoError(t, err)

	other := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.HTTP{Name: "notify", Body: slackBody}}
	_, err = m.Update(ctx, acme, created.ID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, created.ID), Steps: new(automationstep.Root(other))})

	require.ErrorIs(t, err, automations.ErrInvalidStep)
}
