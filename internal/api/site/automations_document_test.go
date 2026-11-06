package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func docStep(subject string, doc map[string]jx.Raw) siteapi.SiteAutomationStep {
	return siteapi.SiteAutomationStep{
		Type:    siteapi.SiteAutomationStepTypeEmail,
		Subject: siteapi.NewOptString(subject),
		Doc:     siteapi.NewOptEditorDocument(doc),
	}
}

func TestSiteAutomationEmailStepsSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Authored", TriggerEvent: "authored.signup",
		Steps: []siteapi.SiteAutomationStep{
			docStep("Hi {{ first_name }}", paragraphDoc("Hello {{ first_name }}")),
			{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)},
			{Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Plain"), Body: siteapi.NewOptString("<p>HTML only</p>")},
		},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	require.Len(t, res.Draft.Value.Steps, 3)

	assert.Contains(t, res.Draft.Value.Steps[0].Body.Value, "Hello {{ first_name }}")
	assert.True(t, res.Draft.Value.Steps[0].Doc.Set, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), res.Draft.Value.Steps[0].EditorVersion.Value)
	assert.False(t, res.Draft.Value.Steps[2].Doc.Set, "a step with no document stays HTML-only")

	stored, err := automationstep.Decode(testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID)))
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(stored[0].Doc))
	assert.Equal(t, res.Draft.Value.Steps[0].Body.Value, stored[0].Body)
	assert.Equal(t, emailrender.SanitizeHTML(stored[0].Body), stored[0].Body, "the stored body survives the sanitizer")
	assert.Empty(t, stored[2].Doc)

	// Saving another document rebuilds that step's body.
	updated, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Steps: []siteapi.SiteAutomationStep{docStep("Hi", paragraphDoc("Second"))},
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	upd := updated
	assert.Contains(t, upd.Draft.Value.Steps[0].Body.Value, "Second")
	assert.NotContains(t, upd.Draft.Value.Steps[0].Body.Value, "Hello")

	// A body-only write to the step clears its document.
	cleared, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Steps: []siteapi.SiteAutomationStep{{
			Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString("<p>Plain now</p>"),
		}},
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	clr := cleared
	assert.False(t, clr.Draft.Value.Steps[0].Doc.Set)
	assert.False(t, clr.Draft.Value.Steps[0].EditorVersion.Set)
	stored, err = automationstep.Decode(testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID)))
	require.NoError(t, err)
	assert.Empty(t, stored[0].Doc)
	assert.Empty(t, stored[0].EditorVersion)
}

func TestSiteAutomationInvalidDocumentRefusesTheWholeSaveNamingTheStep(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	bad := map[string]jx.Raw{"type": jx.Raw(`"doc"`), "content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`)}
	_, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Refused", TriggerEvent: "refused.signup",
		Steps: []siteapi.SiteAutomationStep{
			docStep("Fine", paragraphDoc("Fine")),
			docStep("Broken", bad),
		},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "step 2")

	// Both a body and a document on one step is refused too.
	both := docStep("Both", paragraphDoc("x"))
	both.Body = siteapi.NewOptString("<p>x</p>")
	_, err = c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name: "Both", TriggerEvent: "both.signup", Steps: []siteapi.SiteAutomationStep{both},
	}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	n, err := env.DB.Automation.Query().Count(ctx)
	require.NoError(t, err)
	listed := n
	_, err = c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{Name: "Probe", TriggerEvent: "probe.signup"}, siteapi.SiteAutomationsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.Equal(t, listed+1, env.DB.Automation.Query().CountX(ctx), "the refused saves stored nothing")
}
