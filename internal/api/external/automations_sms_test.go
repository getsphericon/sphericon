package external_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SMS step on /api (ADR 0016, ADR 0081): it is written and read back with its text and the
// Integration it sends through, Publish arms it, and an Integration it sends through cannot be
// archived while the Automation is live.
func TestExternalAnSMSStepIsPublishedAndKeepsItsIntegrationInUse(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	twilio := env.ConnectTwilio(t).Integration.ID
	c := env.ExternalScoped(t, "automations:read", "automations:write", "automations:publish", "integrations:read", "integrations:write")

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:         "Cart reminder",
		TriggerEvent: "cart_abandoned",
		Steps: []externalapi.AutomationStep{{
			Type:          externalapi.AutomationStepTypeSMS,
			Text:          externalapi.NewOptString("Your cart is waiting, {{ first_name }}"),
			IntegrationId: externalapi.NewOptNilEntityId(externalapi.EntityId(twilio)),
		}},
	})
	require.NoError(t, err)
	a, ok := created.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", created)
	require.Len(t, a.Draft.Value.Steps, 1)
	step := a.Draft.Value.Steps[0]
	assert.Equal(t, externalapi.AutomationStepTypeSMS, step.Type)
	assert.Equal(t, "Your cart is waiting, {{ first_name }}", step.Text.Or(""))
	assert.Equal(t, externalapi.EntityId(twilio), step.IntegrationId.Value)
	assert.False(t, step.Subject.Set, "an SMS step carries no email field")

	published, err := c.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: a.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.AutomationResource{}, published)

	archived, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: externalapi.EntityId(twilio)})
	require.NoError(t, err)
	users := usersOfRefusal(t, archived)
	assert.Len(t, automationUsers(users, int64(a.ID)), 1, "the live SMS step sends through it")
}

func TestExternalAnSMSStepWithNoTextIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")

	res, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Blank", TriggerEvent: "cart_abandoned",
		Steps: []externalapi.AutomationStep{{Type: externalapi.AutomationStepTypeSMS}},
	})

	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsCreateUnprocessableEntity{}, res)
}
