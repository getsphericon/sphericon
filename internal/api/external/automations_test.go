package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixture automation AutomationWelcomeSeries is active with email, wait, email steps live and a
// draft that rewords the first email; AutomationTagOnEngagement carries apply_tag and remove_tag
// steps.
func TestExternalAutomationsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	listed, err := c.AutomationsList(ctx, externalapi.AutomationsListParams{})
	require.NoError(t, err)
	total, err := env.DB.Automation.Query().Where(automation.WorkspaceID(fixtures.AcmeID), automation.ArchivedAtIsNil()).Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, total, listed.TotalItems)

	a, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)
	assert.Equal(t, "Welcome series", a.Name)
	assert.Equal(t, externalapi.AutomationStatusActive, a.Status)
	live, draft := a.Live.Value, a.Draft.Value
	require.Len(t, live.Steps.Steps, 3)
	assert.EqualValues(t, 1, live.Number)
	assert.Equal(t, "signup", live.Trigger.OneOf.EventTrigger.Action)
	assert.Equal(t, "Welcome to CodeBasics", live.Steps.Steps[0].OneOf.EmailStep.Subject)
	assert.Equal(t, externalapi.WaitStepAutomationStepSum, live.Steps.Steps[1].OneOf.Type)
	assert.EqualValues(t, 86400, live.Steps.Steps[1].OneOf.WaitStep.Seconds)
	assert.EqualValues(t, 2, draft.Number, "the draft is the version above live")
	assert.Equal(t, "Welcome aboard CodeBasics", draft.Steps.Steps[0].OneOf.EmailStep.Subject)

	tagged, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: entityIDString(fixtures.AutomationTagOnEngagementID)})
	require.NoError(t, err)
	steps := tagged.Live.Value.Steps.Steps
	require.Len(t, steps, 2)
	assert.Equal(t, externalapi.ApplyTagStepAutomationStepSum, steps[0].OneOf.Type)
	assert.Equal(t, "engaged", steps[0].OneOf.ApplyTagStep.Tag)
	assert.Equal(t, externalapi.RemoveTagStepAutomationStepSum, steps[1].OneOf.Type)

	_, err = c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	denied := env.ExternalScoped(t, apitokens.ScopeContactsRead)
	_, err = denied.AutomationsList(ctx, externalapi.AutomationsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// Created automations are always inactive, even for a token that may publish; steps
// are validated; update and delete are workspace-scoped.
func TestExternalAutomationsCreateUpdateDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite, apitokens.ScopeAutomationsPublish)

	a, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    "Tag newcomers",
		Trigger: testhelper.ExternalTrigger("newcomer"),
		Steps: testhelper.ExternalPathEdit(
			externalapi.ApplyTagStepInput{Tag: "newcomer"},
			externalapi.WaitStepInput{Seconds: 60},
		),
	})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusInactive, a.Status)
	assert.True(t, a.Live.Null, "nothing is live until it is published")
	require.Len(t, a.Draft.Value.Steps.Steps, 2)
	assert.Equal(t, "newcomer", a.Draft.Value.Steps.Steps[0].OneOf.ApplyTagStep.Tag)

	_, err = c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Bad", Trigger: testhelper.ExternalTrigger("x"),
		Steps: testhelper.ExternalPathEdit(externalapi.ApplyTagStepInput{Tag: "  "}),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	updated, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(a.ID))),
		Name:  externalapi.NewOptName("Tag newcomers 2"),
		Steps: testhelper.ExternalPathEdit(externalapi.RemoveTagStepInput{Tag: "vip"}),
	}, externalapi.AutomationsUpdateParams{ID: a.ID})
	require.NoError(t, err)
	assert.Equal(t, "Tag newcomers 2", updated.Name)
	require.Len(t, updated.Draft.Value.Steps.Steps, 1)
	assert.Equal(t, externalapi.RemoveTagStepAutomationStepSum, updated.Draft.Value.Steps.Steps[0].OneOf.Type)

	_, err = c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(a.ID))),
		Steps: testhelper.ExternalPathEdit(externalapi.WaitStepInput{Seconds: -1}),
	}, externalapi.AutomationsUpdateParams{ID: a.ID})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: a.ID})
	require.NoError(t, err, "a never-published automation is archived")
	_, err = c.AutomationsArchive(ctx, externalapi.AutomationsArchiveParams{ID: a.ID})
	require.NoError(t, err, "archiving it again changes nothing")

	readOnly := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)
	_, err = readOnly.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{Name: "x", Trigger: testhelper.ExternalTrigger("y")})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// Publish needs automations:publish, which authoring scopes never imply (ADR 0016, ADR 0065).
// AutomationWinBack is inactive.
func TestExternalAutomationsPublishNeedsThePublishScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.AutomationWinBackID)

	authoring := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	_, err := authoring.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(id)))}, externalapi.AutomationsPublishParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	still, err := authoring.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusInactive, still.Status)

	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsPublish)
	on, err := c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(id)))}, externalapi.AutomationsPublishParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusActive, on.Status)
	read, err := c.AutomationsGet(ctx, externalapi.AutomationsGetParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusActive, read.Status)

	_, err = c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(999999)))}, externalapi.AutomationsPublishParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

// Deactivate only stops new Enrollments, so it is authoring: automations:write is enough, and the
// publish scope alone is not. AutomationWelcomeSeries is active.
func TestExternalAutomationsDeactivateNeedsOnlyTheWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := entityIDString(fixtures.AutomationWelcomeSeriesID)

	publisher := env.ExternalScoped(t, apitokens.ScopeAutomationsPublish)
	_, err := publisher.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	authoring := env.ExternalScoped(t, apitokens.ScopeAutomationsWrite)
	off, err := authoring.AutomationsDeactivate(ctx, externalapi.AutomationsDeactivateParams{ID: id})
	require.NoError(t, err)
	assert.Equal(t, externalapi.AutomationStatusInactive, off.Status)
}
