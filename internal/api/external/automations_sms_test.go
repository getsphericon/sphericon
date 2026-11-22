package external_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An SMS step on /api (ADR 0016, ADR 0081): it is written and read back with its text and the
// Integration it sends through, Publish arms it, and an Integration it sends through cannot be
// archived while the Automation is live.
func TestExternalAnSMSStepIsPublishedAndKeepsItsIntegrationInUse(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	twilio := env.AcmeTwilio(t).Integration.ID
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite, apitokens.ScopeAutomationsPublish, apitokens.ScopeIntegrationsRead, apitokens.ScopeIntegrationsWrite)

	a, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:         "Cart reminder",
		TriggerEvent: "cart_abandoned",
		Steps: testhelper.ExternalPathEdit(externalapi.SmsStepInput{
			Text:          "Your cart is waiting, {{ first_name }}",
			IntegrationId: externalapi.NewOptNilEntityId(externalapi.EntityId(twilio)),
		}),
	})
	require.NoError(t, err)
	require.Len(t, a.Draft.Value.Steps.Steps, 1)
	read := a.Draft.Value.Steps.Steps[0].OneOf
	require.Equal(t, externalapi.SmsStepAutomationStepSum, read.Type)
	step := read.SmsStep
	assert.Equal(t, "Your cart is waiting, {{ first_name }}", step.Text)
	assert.Equal(t, externalapi.EntityId(twilio), step.IntegrationId.Value)
	assert.True(t, step.LinkTracking, "links are tracked unless the step turns it off")

	_, err = c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(a.ID)))}, externalapi.AutomationsPublishParams{ID: a.ID})
	require.NoError(t, err)

	err = c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: externalapi.EntityId(twilio)})
	users := usersOfRefusal(t, err)
	assert.Len(t, automationUsers(users, int64(a.ID)), 1, "the live SMS step sends through it")
}

func TestExternalAnSMSStepWithNoTextIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)

	_, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Blank", TriggerEvent: "cart_abandoned",
		Steps: testhelper.ExternalPathEdit(externalapi.SmsStepInput{Text: "  "}),
	})

	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}
