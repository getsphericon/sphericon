package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteAutomationsCRUDPublishAndDeactivate(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name:         "Welcome series",
		TriggerEvent: systemevent.ContactCreated,
		Steps: testhelper.SitePathEdit(
			siteapi.EmailStepInput{Subject: "Hi", Body: siteapi.NewOptString("<p></p>")},
			siteapi.WaitStepInput{Seconds: 3600},
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	assert.Equal(t, "Welcome series", res.Name)
	assert.Equal(t, siteapi.AutomationStatusInactive, res.Status)
	assert.Equal(t, systemevent.ContactCreated, res.Draft.Value.TriggerEvent)

	// Steps round-trip through the typed contract (stored as the executor's JSON
	// string, decoded back into the typed DTO).
	steps := res.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	assert.Equal(t, siteapi.EmailStepAutomationStepSum, steps[0].OneOf.Type)
	assert.Equal(t, "Hi", steps[0].OneOf.EmailStep.Subject)
	assert.Equal(t, siteapi.WaitStepAutomationStepSum, steps[1].OneOf.Type)
	assert.Equal(t, int32(3600), steps[1].OneOf.WaitStep.Seconds)

	// Publish / deactivate flip the status.
	act, err := c.SiteAutomationsPublish(ctx, &siteapi.RevisionInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID)))}, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	actRes := act
	assert.Equal(t, siteapi.AutomationStatusActive, actRes.Status)

	deact, err := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	deactRes := deact
	assert.Equal(t, siteapi.AutomationStatusInactive, deactRes.Status)

	// Fetch the created automation back by id (selection by key).
	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	gotRes := got
	assert.Equal(t, res.ID, gotRes.ID)

	// Update renames.
	upd, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))),
		Name: siteapi.NewOptName("Onboarding"),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	updRes := upd
	assert.Equal(t, "Onboarding", updRes.Name)

	// Archive: it is kept, and a fetch by id still reads it, archived.
	_, err = c.SiteAutomationsArchive(ctx, siteapi.SiteAutomationsArchiveParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)

	kept, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	keptRes := kept
	assert.True(t, keptRes.ArchivedAt.IsSet())
}
