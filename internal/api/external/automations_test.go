package external_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/automation"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture automation AutomationWelcomeSeries is active with email, wait, email steps;
// AutomationTagOnEngagement carries apply_tag and remove_tag steps.
func TestExternalAutomationsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read")

	list, err := c.AutomationsList(ctx, externalapi.AutomationsListParams{})
	require.NoError(t, err)
	listed, ok := list.(*externalapi.AutomationsListOK)
	require.Truef(t, ok, "got %T", list)
	total, err := env.DB.Automation.Query().Where(automation.WorkspaceID(fixtures.AcmeID), automation.DiscardedAtIsNil()).Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, total, listed.TotalItems)

	got, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	a, ok := got.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, "Welcome series", a.Name)
	assert.Equal(t, externalapi.AutomationStatusActive, a.Status)
	require.Len(t, a.Steps, 3)
	assert.Equal(t, externalapi.AutomationStepTypeWait, a.Steps[1].Type)
	assert.EqualValues(t, 86400, a.Steps[1].Seconds.Or(0))

	tagged, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: entityIDString(fixtures.AutomationTagOnEngagementID)})
	require.NoError(t, err)
	steps := tagged.(*externalapi.AutomationResource).Steps
	require.Len(t, steps, 2)
	assert.Equal(t, externalapi.AutomationStepTypeApplyTag, steps[0].Type)
	assert.Equal(t, "engaged", steps[0].Tag.Or(""))
	assert.Equal(t, externalapi.AutomationStepTypeRemoveTag, steps[1].Type)

	missing, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsGetNotFound{}, missing)

	denied := env.ExternalScoped(t, "contacts:read")
	res, err := denied.AutomationsList(ctx, externalapi.AutomationsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsListForbidden{}, res)
}

// Created automations are always draft, even for a token that may activate; steps
// are validated; update and delete are workspace-scoped.
func TestExternalAutomationsCreateUpdateDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write", "automations:activate")

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:         "Tag newcomers",
		TriggerEvent: "newcomer",
		Steps: []externalapi.AutomationStep{
			{Type: externalapi.AutomationStepTypeApplyTag, Tag: externalapi.NewOptString("newcomer")},
			{Type: externalapi.AutomationStepTypeWait, Seconds: externalapi.NewOptInt32(60)},
		},
	})
	require.NoError(t, err)
	a, ok := created.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, externalapi.AutomationStatusDraft, a.Status)
	require.Len(t, a.Steps, 2)
	assert.Equal(t, "newcomer", a.Steps[0].Tag.Or(""))

	blank, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Bad", TriggerEvent: "x",
		Steps: []externalapi.AutomationStep{{Type: externalapi.AutomationStepTypeApplyTag, Tag: externalapi.NewOptString("  ")}},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsCreateUnprocessableEntity{}, blank)

	upd, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		Name:  externalapi.NewOptString("Tag newcomers 2"),
		Steps: []externalapi.AutomationStep{{Type: externalapi.AutomationStepTypeRemoveTag, Tag: externalapi.NewOptString("vip")}},
	}, externalapi.AutomationsUpdateParams{ID: a.ID})
	require.NoError(t, err)
	updated := upd.(*externalapi.AutomationResource)
	assert.Equal(t, "Tag newcomers 2", updated.Name)
	require.Len(t, updated.Steps, 1)
	assert.Equal(t, externalapi.AutomationStepTypeRemoveTag, updated.Steps[0].Type)

	badUpd, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		Steps: []externalapi.AutomationStep{{Type: externalapi.AutomationStepTypeApplyTag}},
	}, externalapi.AutomationsUpdateParams{ID: a.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsUpdateUnprocessableEntity{}, badUpd)

	del, err := c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: a.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsDeleteNoContent{}, del)
	again, err := c.AutomationsDelete(ctx, externalapi.AutomationsDeleteParams{ID: a.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsDeleteNotFound{}, again)

	readOnly := env.ExternalScoped(t, "automations:read")
	res, err := readOnly.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "x", TriggerEvent: "y"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsCreateForbidden{}, res)
}

// Activate and deactivate need automations:activate, which authoring scopes never
// imply (ADR 0016). AutomationWinBack is a draft.
func TestExternalAutomationsActivateAndDeactivate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	authoring := env.ExternalScoped(t, "automations:read", "automations:write")
	refused, err := authoring.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: entityIDString(fixtures.AutomationWinBackID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsActivateForbidden{}, refused)
	refusedOff, err := authoring.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsDeactivateForbidden{}, refusedOff)
	assert.Equal(t, automation.StatusDraft, env.DB.Automation.GetX(ctx, fixtures.AutomationWinBackID).Status)

	c := env.ExternalScoped(t, "automations:activate")
	on, err := c.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: entityIDString(fixtures.AutomationWinBackID)})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusActive, on.(*externalapi.AutomationResource).Status)
	assert.Equal(t, automation.StatusActive, env.DB.Automation.GetX(ctx, fixtures.AutomationWinBackID).Status)

	off, err := c.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: entityIDString(fixtures.AutomationWinBackID)})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusDraft, off.(*externalapi.AutomationResource).Status)

	missing, err := c.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsActivateNotFound{}, missing)
}
