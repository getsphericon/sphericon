package external_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The external API takes the editor's document under the same rules as the site (ADR 0040).

func paragraphDoc(text string) map[string]jx.Raw {
	return map[string]jx.Raw{
		"type": jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"paragraph","content":[{"type":"text","text":` +
			strconv.Quote(text) + `}]}]`),
	}
}

func badHeadingDoc() map[string]jx.Raw {
	return map[string]jx.Raw{
		"type":    jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`),
	}
}

func TestExternalTemplatesSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead, apitokens.ScopeTemplatesWrite)
	ctx := context.Background()

	res, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name:    "Authored",
		Subject: externalapi.NewOptString("Hi {{ first_name }}"),
		Doc:     externalapi.NewOptEditorDocument(paragraphDoc("Hello {{ first_name }}")),
	})
	require.NoError(t, err)
	assert.Contains(t, res.Body, "Hello {{ first_name }}")
	assert.True(t, res.Doc.Set && !res.Doc.Null, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), res.EditorVersion.Value)

	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Equal(t, res.Body, row.Body)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(row.Doc))

	// A read returns the document: an agent copies a Template by reading it and writing a new one.
	read, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: res.ID})
	require.NoError(t, err)
	want, err := json.Marshal(res.Doc.Value)
	require.NoError(t, err)
	have, err := json.Marshal(read.Doc.Value)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(have))

	// Saving another document rebuilds the body.
	upd, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{
		Revision: read.Revision,
		Doc:      externalapi.NewOptEditorDocument(paragraphDoc("Second")),
	}, externalapi.TemplatesUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.Contains(t, upd.Body, "Second")
	assert.NotContains(t, upd.Body, "Hello")
}

func TestExternalTemplateBodyOnlyWriteStoresNoDocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead, apitokens.ScopeTemplatesWrite)
	ctx := context.Background()

	res, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Html", Body: externalapi.NewOptString("<p>Hi</p>"),
	})
	require.NoError(t, err)
	assert.False(t, res.Doc.Set && !res.Doc.Null, "a body sent as HTML has no document")
	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)

	// A body written over a document clears it.
	id := entityIDString(fixtures.TemplateAuthoredID)
	authored := env.DB.EmailTemplate.GetX(ctx, int64(id))
	clr, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: externalapi.Revision(authored.Revision), Body: externalapi.NewOptString("<p>Plain</p>")},
		externalapi.TemplatesUpdateParams{ID: id})
	require.NoError(t, err)
	assert.False(t, clr.Doc.Set && !clr.Doc.Null)
	row = env.DB.EmailTemplate.GetX(ctx, int64(id))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
	assert.Equal(t, "<p>Plain</p>", row.Body)
}

func TestExternalTemplatesRefuseAnInvalidDocumentWithTheReasonAndStoreNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead, apitokens.ScopeTemplatesWrite)
	ctx := context.Background()
	before := env.DB.EmailTemplate.Query().CountX(ctx)

	_, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Bad", Doc: externalapi.NewOptEditorDocument(badHeadingDoc()),
	})
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "level")

	_, err = c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Both", Body: externalapi.NewOptString("<p>x</p>"),
		Doc: externalapi.NewOptEditorDocument(paragraphDoc("x")),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	// A document written under an injection is refused as any other invalid one: it names
	// what is wrong with the document, never repairs it.
	hostile := map[string]jx.Raw{
		"type": jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[` +
			`{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]`),
	}
	_, err = c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Hostile", Doc: externalapi.NewOptEditorDocument(hostile),
	})
	hostileRefused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, hostileRefused.Detail.Or(""), "absolute URL")

	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing is stored")
}

func TestExternalBroadcastsSaveADocumentAndRefuseAnInvalidOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeBroadcastsRead, apitokens.ScopeBroadcastsWrite)
	ctx := context.Background()

	res, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:    "Authored",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Subject: externalapi.NewOptString("Hi"), Doc: externalapi.NewOptEditorDocument(paragraphDoc("Hello {{ first_name }}"))}),
	})
	require.NoError(t, err)
	assert.Contains(t, testhelper.ExternalEmailOf(res).Body, "Hello {{ first_name }}")
	assert.True(t, testhelper.ExternalEmailOf(res).Doc.Set && !testhelper.ExternalEmailOf(res).Doc.Null)
	assert.Equal(t, emaildoc.EditorVersion(), testhelper.ExternalEmailOf(res).EditorVersion.Value)
	row := env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Equal(t, testhelper.ExternalEmailOf(res).Body, *row.Body)
	assert.NotEmpty(t, row.Doc)

	// The edits go to a draft written in the editor.
	authored := externalapi.BroadcastsUpdateParams{ID: entityIDString(fixtures.BroadcastAuthoredID)}
	stored := *env.DB.Broadcast.GetX(ctx, fixtures.BroadcastAuthoredID).Body
	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(authored.ID))),
		Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{Doc: externalapi.NewOptEditorDocument(badHeadingDoc())}),
	}, authored)
	r := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, r.Detail.Or(""), "level")
	assert.Equal(t, stored, *env.DB.Broadcast.GetX(ctx, fixtures.BroadcastAuthoredID).Body, "nothing is stored")

	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(authored.ID))),
		Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptEditorDocument(paragraphDoc("x"))}),
	}, authored)
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Revision: externalapi.Revision(env.BroadcastRevision(ctx, t, int64(authored.ID))), Content: testhelper.ExternalEmailEdit(externalapi.EmailBroadcastContentInput{Body: externalapi.NewOptString("<p>Plain</p>")})},
		authored)
	require.NoError(t, err)
	assert.Empty(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastAuthoredID).Doc)
}

// An external read-then-write keeps an email step's document: the step carries it on read, and
// writing the document alone (as the site does) round-trips.
func TestExternalAutomationEmailStepsCarryTheDocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()

	res, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Authored", TriggerEvent: "authored.signup",
		Steps: []externalapi.AutomationStep{{
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString("Hi"),
			Doc:     externalapi.NewOptEditorDocument(paragraphDoc("Hello {{ first_name }}")),
		}},
	})
	require.NoError(t, err)
	step := res.Draft.Value.Steps[0]
	assert.Contains(t, step.Body.Value, "Hello {{ first_name }}")
	assert.True(t, step.Doc.Set, "the step carries its document")
	assert.Equal(t, emaildoc.EditorVersion(), step.EditorVersion.Value)

	// Read, change the document, write it back (without the body).
	step.Body = externalapi.OptString{}
	step.EditorVersion = externalapi.OptString{}
	step.Doc = externalapi.NewOptEditorDocument(paragraphDoc("Second"))
	upd, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))), Steps: []externalapi.AutomationStep{step}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.Contains(t, upd.Draft.Value.Steps[0].Body.Value, "Second")
	assert.True(t, upd.Draft.Value.Steps[0].Doc.Set)

	// Writing the body with the document is refused, naming the step.
	_, err = c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))), Steps: []externalapi.AutomationStep{upd.Draft.Value.Steps[0]}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "step 1")

	// An invalid document refuses the save; the stored step is unchanged.
	bad := upd.Draft.Value.Steps[0]
	bad.Body = externalapi.OptString{}
	bad.Doc = externalapi.NewOptEditorDocument(badHeadingDoc())
	_, err = c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(res.ID))), Steps: []externalapi.AutomationStep{bad}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	stored, err := automationstep.Decode(testhelper.LatestSteps(ctx, t, env.DB, int64(res.ID)))
	require.NoError(t, err)
	assert.Contains(t, stored[0].Body, "Second")
}

func TestExternalEmailLintTakesADocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesRead)
	ctx := context.Background()

	res, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{Doc: externalapi.NewOptEditorDocument(paragraphDoc("Hi {{ first_name }}"))})
	require.NoError(t, err)
	assert.Empty(t, res.Errors)

	got, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{Doc: externalapi.NewOptEditorDocument(badHeadingDoc())})
	require.NoError(t, err)
	require.Len(t, got.Errors, 1)
	assert.Contains(t, got.Errors[0].Detail.Or(""), "level")

	_, err = c.EmailLintLint(ctx, &externalapi.LintEmailInput{
		Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptEditorDocument(paragraphDoc("x")),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.EmailLintLint(ctx, &externalapi.LintEmailInput{})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}
