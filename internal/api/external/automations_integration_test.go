package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// emailStepVia is an email step through the Integration id names; an unset one is the default.
func emailStepVia(id externalapi.OptNilEntityId) externalapi.EmailStepInput {
	return externalapi.EmailStepInput{Subject: "Hi", Body: externalapi.NewOptString("<p>Hi</p>"), IntegrationId: id}
}

// /api names the Integration of each send step like /site does: saved with the step, read back on
// it, none for the default, refused with a 422 when it is not the Workspace's (ADR 0081).
func TestExternalAutomationStepsNameTheirIntegration(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()
	mailgun := externalapi.NewOptNilEntityId(entityIDString(fixtures.IntegrationAcmeMailgunID))

	res, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Onboarding", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: testhelper.ExternalPathEdit(emailStepVia(mailgun), emailStepVia(externalapi.OptNilEntityId{})),
	})
	require.NoError(t, err)
	steps := res.Draft.Value.Steps.Steps

	bad := externalapi.NewOptNilEntityId(entityIDString(fixtures.IntegrationGlobexID)) // another Workspace's
	_, refusedCreate := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "x", Trigger: testhelper.ExternalTrigger("signup"), Steps: testhelper.ExternalPathEdit(emailStepVia(bad)),
	})
	_, refusedUpdate := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))), Steps: testhelper.ExternalPathEdit(emailStepVia(bad))},
		externalapi.AutomationsUpdateParams{ID: res.ID})

	require.Len(t, steps, 2)
	assert.EqualValues(t, fixtures.IntegrationAcmeMailgunID, steps[0].OneOf.EmailStep.IntegrationId.Or(0), "the first step through the relay")
	assert.False(t, steps[1].OneOf.EmailStep.IntegrationId.Set, "the second step names none: the default")
	testhelper.ExternalRefusal(t, refusedCreate, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, refusedUpdate, http.StatusUnprocessableEntity)
}
