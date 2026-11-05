package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalHTTPStepIsMaskedOnRead(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")
	ctx := context.Background()

	res, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Notify", TriggerEvent: "signup",
		Steps: []externalapi.AutomationStep{{
			Type: externalapi.AutomationStepTypeHTTP, Body: externalapi.NewOptString(`{"a":1}`),
			URL:     externalapi.NewOptString("https://hooks.example.com/SECRETPATH"),
			Headers: []externalapi.AutomationHttpHeader{{Name: "X-Key", Value: externalapi.NewOptString("hunter2")}},
			Secret:  externalapi.NewOptString("whsec_x"),
		}},
	})
	require.NoError(t, err)
	step := res.Draft.Value.Steps[0]
	assert.True(t, step.HasSecret.Or(false))
	assert.False(t, step.URL.Set)
	assert.False(t, step.Secret.Set)
	require.Len(t, step.Headers, 1)
	assert.False(t, step.Headers[0].Value.Set)
}

func TestExternalHTTPStepDryRunNeedsAutomationsWriteAndContactsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	in := &externalapi.DryRunHttpStepInput{Body: `{"name":"{{ first_name }}"}`, ContactId: externalapi.EntityId(fixtures.ContactAliceID)}

	_, err := env.ExternalScoped(t, "automations:write").AutomationsDryRunHttpStep(ctx, in)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	res, err := env.ExternalScoped(t, "automations:write", "contacts:read").AutomationsDryRunHttpStep(ctx, in)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Alice"}`, res.Body.Or(""))
}
