package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const endStepID = "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e71"

// An End is a Step of its own with no fields but its id (ADR 0124): a version that ends its path
// with one keeps it through a save, a read and the next save that sends it back.
func TestSiteAutomationKeepsAnEndThatFinishesItsPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Ends", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(
			siteapi.WaitStepInput{Seconds: 60},
			siteapi.EndStepInput{ID: siteapi.NewOptStepId(endStepID)},
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)

	resent := testhelper.SitePath(
		siteapi.ApplyTagStepInput{Tag: "ending"},
		siteapi.EndStepInput{ID: siteapi.NewOptStepId(endStepID)},
	)
	resent.ID = siteapi.NewOptStepId(created.Draft.Value.Steps.ID)
	_, err = c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{
		Revision: created.Revision, Steps: siteapi.NewOptAutomationPathInput(resent),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)
	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)

	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	assert.Equal(t, siteapi.EndStepAutomationStepSum, steps[1].OneOf.Type)
	assert.Equal(t, []string{siteStepIDs(got.Draft.Value.Steps)[0], endStepID}, siteStepIDs(got.Draft.Value.Steps))
}

// An End finishes its path, so a Step after it in the same path refuses the save, naming that
// Step by its id and kind.
func TestSiteAutomationRefusesAStepAfterAnEnd(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	after := "0f6b8d1e-6c2a-4a55-8f7e-3c1d2b4a5e72"

	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{
		Name: "After the end", Trigger: testhelper.SiteTrigger("signup"),
		Steps: testhelper.SitePathEdit(
			siteapi.EndStepInput{},
			siteapi.ApplyTagStepInput{ID: siteapi.NewOptStepId(siteapi.StepId(after)), Tag: "never"},
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	detail := refused.Detail.Or("")
	assert.Contains(t, detail, after)
	assert.Contains(t, detail, "apply_tag")
}
