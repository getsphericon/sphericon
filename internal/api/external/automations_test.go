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

// Fixture automation AutomationWelcomeSeries is active with email, wait, email steps live and a
// draft that rewords the first email; AutomationTagOnEngagement carries apply_tag and remove_tag
// steps.
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
	live, draft := a.Live.Value, a.Draft.Value
	require.Len(t, live.Steps, 3)
	assert.EqualValues(t, 1, live.Number)
	assert.Equal(t, "signup", live.TriggerEvent)
	assert.Equal(t, "Welcome to CodeBasics", live.Steps[0].Subject.Or(""))
	assert.Equal(t, externalapi.AutomationStepTypeWait, live.Steps[1].Type)
	assert.EqualValues(t, 86400, live.Steps[1].Seconds.Or(0))
	assert.EqualValues(t, 2, draft.Number, "the draft is the version above live")
	assert.Equal(t, "Welcome aboard CodeBasics", draft.Steps[0].Subject.Or(""))

	tagged, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: entityIDString(fixtures.AutomationTagOnEngagementID)})
	require.NoError(t, err)
	steps := tagged.(*externalapi.AutomationResource).Live.Value.Steps
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

// Created automations are always inactive, even for a token that may publish; steps
// are validated; update and delete are workspace-scoped.
func TestExternalAutomationsCreateUpdateDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "automations:read", "automations:write", "automations:publish")

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
	assert.Equal(t, externalapi.AutomationStatusInactive, a.Status)
	assert.True(t, a.Live.Null, "nothing is live until it is published")
	require.Len(t, a.Draft.Value.Steps, 2)
	assert.Equal(t, "newcomer", a.Draft.Value.Steps[0].Tag.Or(""))

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
	require.Len(t, updated.Draft.Value.Steps, 1)
	assert.Equal(t, externalapi.AutomationStepTypeRemoveTag, updated.Draft.Value.Steps[0].Type)

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

// Publish needs automations:publish, which authoring scopes never imply (ADR 0016, ADR 0065).
// AutomationWinBack is inactive.
func TestExternalAutomationsPublishNeedsThePublishScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.AutomationWinBackID)

	authoring := env.ExternalScoped(t, "automations:read", "automations:write")
	refused, err := authoring.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsPublishForbidden{}, refused)
	still, err := authoring.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusInactive, still.(*externalapi.AutomationResource).Status)

	c := env.ExternalScoped(t, "automations:read", "automations:publish")
	on, err := c.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusActive, on.(*externalapi.AutomationResource).Status)
	read, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusActive, read.(*externalapi.AutomationResource).Status)

	missing, err := c.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsPublishNotFound{}, missing)
}

// Deactivate only stops new Enrollments, so it is authoring: automations:write is enough, and the
// publish scope alone is not. AutomationWelcomeSeries is active.
func TestExternalAutomationsDeactivateNeedsOnlyTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.AutomationWelcomeSeriesID)

	publisher := env.ExternalScoped(t, "automations:publish")
	refused, err := publisher.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsDeactivateForbidden{}, refused)

	authoring := env.ExternalScoped(t, "automations:write")
	off, err := authoring.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusInactive, off.(*externalapi.AutomationResource).Status)
}
