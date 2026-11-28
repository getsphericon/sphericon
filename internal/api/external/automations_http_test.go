package external_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalHTTPStepIsMaskedOnRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := t.Context()

	res, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Notify", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: testhelper.ExternalPathEdit(externalapi.HttpStepInput{
			Name:    "notify",
			Body:    `{"a":1}`,
			URL:     externalapi.NewOptString("https://hooks.example.com/SECRETPATH"),
			Headers: []externalapi.AutomationHttpHeaderInput{{Name: "X-Key", Value: externalapi.NewOptString("hunter2")}},
			Secret:  externalapi.NewOptSigningSecret(externalapi.SigningSecret(testhelper.SigningSecret("http"))),
		}),
	})
	require.NoError(t, err)
	step := res.Draft.Value.Steps.Steps[0].OneOf.HttpStep
	assert.True(t, step.HasSecret)
	require.Len(t, step.Headers, 1)
	assert.False(t, step.Headers[0].Value.Set)
	read := testhelper.JSONOf(t, &res.Draft.Value.Steps)
	assert.NotContains(t, read, "SECRETPATH", "the URL is never returned")
	assert.NotContains(t, read, "hunter2", "a header value is never returned")
}

func TestExternalHTTPStepDryRunNeedsAutomationsWriteAndContactsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	in := &externalapi.DryRunHttpStepInput{Body: `{"name":"{{ first_name }}"}`, ContactId: externalapi.EntityId(fixtures.ContactAliceID)}

	_, err := env.ExternalScoped(t, apitokens.ScopeAutomationsWrite).AutomationsDryRunHttpStep(ctx, in)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	res, err := env.ExternalScoped(t, apitokens.ScopeAutomationsWrite, apitokens.ScopeContactsRead).AutomationsDryRunHttpStep(ctx, in)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Alice"}`, res.Body.Or(""))
}
