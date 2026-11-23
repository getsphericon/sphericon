package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func docStep(subject string, doc map[string]jx.Raw) siteapi.EmailStepInput {
	return siteapi.EmailStepInput{Subject: subject, Doc: siteapi.NewOptEditorDocument(doc)}
}

func TestSiteAutomationEmailStepsSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Authored", Trigger: testhelper.SiteTrigger("authored.signup"),
		Steps: testhelper.SitePathEdit(
			docStep("Hi {{ first_name }}", paragraphDoc("Hello {{ first_name }}")),
			siteapi.WaitStepInput{Seconds: 60},
			siteapi.EmailStepInput{Subject: "Plain", Body: siteapi.NewOptString("<p>HTML only</p>")},
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	steps := res.Draft.Value.Steps.Steps
	require.Len(t, steps, 3)
	authored, plain := steps[0].OneOf.EmailStep, steps[2].OneOf.EmailStep

	assert.Contains(t, authored.Body, "Hello {{ first_name }}")
	assert.True(t, authored.Doc.Set, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), authored.EditorVersion.Value)
	assert.False(t, plain.Doc.Set, "a step with no document stays HTML-only")

	stored := testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID))
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(storedEmail(t, stored, 0).Doc))
	assert.Equal(t, authored.Body, storedEmail(t, stored, 0).Body)
	assert.Empty(t, storedEmail(t, stored, 2).Doc)

	// Saving another document rebuilds that step's body.
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))),
		Steps: testhelper.SitePathEdit(docStep("Hi", paragraphDoc("Second"))),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	upd := updated.Draft.Value.Steps.Steps[0].OneOf.EmailStep
	assert.Contains(t, upd.Body, "Second")
	assert.NotContains(t, upd.Body, "Hello")

	// A body-only write to the step clears its document.
	cleared, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))),
		Steps: testhelper.SitePathEdit(siteapi.EmailStepInput{Subject: "Hi", Body: siteapi.NewOptString("<p>Plain now</p>")}),
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	clr := cleared.Draft.Value.Steps.Steps[0].OneOf.EmailStep
	assert.False(t, clr.Doc.Set)
	assert.False(t, clr.EditorVersion.Set)
	stored = testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID))
	assert.Empty(t, storedEmail(t, stored, 0).Doc)
	assert.Empty(t, storedEmail(t, stored, 0).EditorVersion)
}

func TestSiteAutomationInvalidDocumentRefusesTheWholeSaveNamingTheStep(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	bad := map[string]jx.Raw{"type": jx.Raw(`"doc"`), "content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`)}
	broken := docStep("Broken", bad)
	broken.ID = siteapi.NewOptStepId("5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b02")
	_, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Refused", Trigger: testhelper.SiteTrigger("refused.signup"),
		Steps: testhelper.SitePathEdit(
			docStep("Fine", paragraphDoc("Fine")),
			broken,
		),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "step 5b0c6a52-4f0e-4f65-9a70-0d4a1c0e1b02 (email)")

	// Both a body and a document on one step is refused too.
	both := docStep("Both", paragraphDoc("x"))
	both.Body = siteapi.NewOptString("<p>x</p>")
	_, err = c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Both", Trigger: testhelper.SiteTrigger("both.signup"), Steps: testhelper.SitePathEdit(both),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	n, err := env.DB.Automation.Query().Count(ctx)
	require.NoError(t, err)
	listed := n
	_, err = c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Probe", Trigger: testhelper.SiteTrigger("probe.signup")}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.Equal(t, listed+1, env.DB.Automation.Query().CountX(ctx), "the refused saves stored nothing")
}

// A document saved again is the one the step holds, though jsonb keeps its keys in another order
// (ADR 0169): the save changes nothing and the Automation's revision stays.
func TestSiteAutomationDocumentSavedAgainIsNoChange(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	step := docStep("Hi", paragraphDoc("Hello {{ first_name }}"))
	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{
		Name: "Saved twice", Trigger: testhelper.SiteTrigger("saved.twice"), Steps: testhelper.SitePathEdit(step),
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	// The same Step saved again, under the ids it was given.
	read := created.Draft.Value.Steps
	step.ID = siteapi.NewOptStepId(read.Steps[0].OneOf.EmailStep.ID)
	again := testhelper.SitePath(step)
	again.ID = siteapi.NewOptStepId(read.ID)

	saved, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: created.Revision, Steps: siteapi.NewOptAutomationPathInput(again)},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: created.ID})

	require.NoError(t, err)
	assert.Equal(t, created.Revision, saved.Revision)
}
