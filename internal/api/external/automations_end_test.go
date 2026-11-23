package external_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An End is a Step of its own with no fields but its id (ADR 0124): a version that ends its path
// with one keeps it through a save and a read.
func TestExternalAutomationKeepsAnEndThatFinishesItsPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, stepAuthorScopes...)

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Ends", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			externalapi.WaitStepInput{Seconds: 60},
			externalapi.EndStepInput{ID: externalapi.NewOptStepId(keptStepID)},
		)),
	})
	require.NoError(t, err)
	got, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: created.ID})
	require.NoError(t, err)

	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	assert.Equal(t, externalapi.EndStepAutomationStepSum, steps[1].OneOf.Type)
	assert.EqualValues(t, keptStepID, steps[1].OneOf.EndStep.ID)
}

// An End finishes its path, so a Step after it in the same path refuses the save, naming that
// Step by its id and kind.
func TestExternalAutomationRefusesAStepAfterAnEnd(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "After the end", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			externalapi.EndStepInput{},
			externalapi.WaitStepInput{ID: externalapi.NewOptStepId(keptStepID), Seconds: 60},
		)),
	})

	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, keptStepID)
	assert.Contains(t, detail, "wait")
}
