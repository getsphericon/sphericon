package external_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
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
	c := env.ExternalScoped(t, "templates:read", "templates:write")
	ctx := context.Background()

	created, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name:    "Authored",
		Subject: externalapi.NewOptString("Hi {{ first_name }}"),
		Doc:     externalapi.NewOptCreateTemplateInputDoc(paragraphDoc("Hello {{ first_name }}")),
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, res.Body, "Hello {{ first_name }}")
	assert.True(t, res.Doc.Set && !res.Doc.Null, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), res.EditorVersion.Value)

	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Equal(t, res.Body, row.Body)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(row.Doc))
	assert.Equal(t, emailrender.SanitizeHTML(row.Body), row.Body, "the stored body survives the sanitizer")

	// A read returns the document: an agent copies a Template by reading it and writing a new one.
	got, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: res.ID})
	require.NoError(t, err)
	read, ok := got.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", got)
	want, err := json.Marshal(res.Doc.Value)
	require.NoError(t, err)
	have, err := json.Marshal(read.Doc.Value)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(have))

	// Saving another document rebuilds the body.
	updated, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{
		Doc: externalapi.NewOptUpdateTemplateInputDoc(paragraphDoc("Second")),
	}, externalapi.TemplatesUpdateParams{ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Contains(t, upd.Body, "Second")
	assert.NotContains(t, upd.Body, "Hello")
}

func TestExternalTemplateBodyOnlyWriteStoresNoDocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read", "templates:write")
	ctx := context.Background()

	html, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Html", Body: externalapi.NewOptString("<p>Hi</p>"),
	})
	require.NoError(t, err)
	res, ok := html.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", html)
	assert.False(t, res.Doc.Set && !res.Doc.Null, "a body sent as HTML has no document")
	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)

	// A body written over a document clears it.
	authored, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Authored", Doc: externalapi.NewOptCreateTemplateInputDoc(paragraphDoc("x")),
	})
	require.NoError(t, err)
	id := authored.(*externalapi.TemplateResource).ID
	cleared, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Body: externalapi.NewOptString("<p>Plain</p>")},
		externalapi.TemplatesUpdateParams{ID: id})
	require.NoError(t, err)
	clr, ok := cleared.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", cleared)
	assert.False(t, clr.Doc.Set && !clr.Doc.Null)
	row = env.DB.EmailTemplate.GetX(ctx, int64(id))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
	assert.Equal(t, "<p>Plain</p>", row.Body)
}

func TestExternalTemplatesRefuseAnInvalidDocumentWithTheReasonAndStoreNothing(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read", "templates:write")
	ctx := context.Background()
	before := env.DB.EmailTemplate.Query().CountX(ctx)

	created, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Bad", Doc: externalapi.NewOptCreateTemplateInputDoc(badHeadingDoc()),
	})
	require.NoError(t, err)
	refused, ok := created.(*externalapi.TemplatesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, refused.Detail.Or(""), "level")

	both, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Both", Body: externalapi.NewOptString("<p>x</p>"),
		Doc: externalapi.NewOptCreateTemplateInputDoc(paragraphDoc("x")),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TemplatesCreateUnprocessableEntity{}, both)

	// A document written under an injection is refused as any other invalid one: it names
	// what is wrong with the document, never repairs it.
	hostile := map[string]jx.Raw{
		"type": jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[` +
			`{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]`),
	}
	bad, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Hostile", Doc: externalapi.NewOptCreateTemplateInputDoc(hostile),
	})
	require.NoError(t, err)
	hostileRefused, ok := bad.(*externalapi.TemplatesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", bad)
	assert.Contains(t, hostileRefused.Detail.Or(""), "absolute URL")

	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing is stored")
}

func TestExternalBroadcastsSaveADocumentAndRefuseAnInvalidOne(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "broadcasts:read", "broadcasts:write")
	ctx := context.Background()

	created, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "Authored", Subject: externalapi.NewOptString("Hi"),
		Doc: externalapi.NewOptCreateBroadcastInputDoc(paragraphDoc("Hello {{ first_name }}")),
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, res.Body, "Hello {{ first_name }}")
	assert.True(t, res.Doc.Set && !res.Doc.Null)
	assert.Equal(t, emaildoc.EditorVersion(), res.EditorVersion.Value)
	row := env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Equal(t, res.Body, row.Body)
	assert.NotEmpty(t, row.Doc)

	refused, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{
		Doc: externalapi.NewOptUpdateBroadcastInputDoc(badHeadingDoc()),
	}, externalapi.BroadcastsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	r, ok := refused.(*externalapi.BroadcastsUpdateUnprocessableEntity)
	require.Truef(t, ok, "got %T", refused)
	assert.Contains(t, r.Detail.Or(""), "level")
	assert.Equal(t, res.Body, env.DB.Broadcast.GetX(ctx, int64(res.ID)).Body, "nothing is stored")

	both, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{
		Body: externalapi.NewOptString("<p>x</p>"),
		Doc:  externalapi.NewOptUpdateBroadcastInputDoc(paragraphDoc("x")),
	}, externalapi.BroadcastsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BroadcastsUpdateUnprocessableEntity{}, both)

	cleared, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Body: externalapi.NewOptString("<p>Plain</p>")},
		externalapi.BroadcastsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.BroadcastResource{}, cleared)
	assert.Empty(t, env.DB.Broadcast.GetX(ctx, int64(res.ID)).Doc)
}

// An external read-then-write keeps an email step's document: the step carries it on read, and
// writing the document alone (as the site does) round-trips.
func TestExternalAutomationEmailStepsCarryTheDocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")
	ctx := context.Background()

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Authored", TriggerEvent: "authored.signup",
		Steps: []externalapi.AutomationStep{{
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString("Hi"),
			Doc:     externalapi.NewOptAutomationStepDoc(paragraphDoc("Hello {{ first_name }}")),
		}},
	})
	require.NoError(t, err)
	res, ok := created.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", created)
	step := res.Steps[0]
	assert.Contains(t, step.Body.Value, "Hello {{ first_name }}")
	assert.True(t, step.Doc.Set, "the step carries its document")
	assert.Equal(t, emaildoc.EditorVersion(), step.EditorVersion.Value)

	// Read, change the document, write it back (without the body).
	step.Body = externalapi.OptString{}
	step.EditorVersion = externalapi.OptString{}
	step.Doc = externalapi.NewOptAutomationStepDoc(paragraphDoc("Second"))
	updated, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Steps: []externalapi.AutomationStep{step}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Contains(t, upd.Steps[0].Body.Value, "Second")
	assert.True(t, upd.Steps[0].Doc.Set)

	// Writing the body with the document is refused, naming the step.
	both, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Steps: []externalapi.AutomationStep{upd.Steps[0]}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	refused, ok := both.(*externalapi.AutomationsUpdateUnprocessableEntity)
	require.Truef(t, ok, "got %T", both)
	assert.Contains(t, refused.Detail.Or(""), "step 1")

	// An invalid document refuses the save; the stored step is unchanged.
	bad := upd.Steps[0]
	bad.Body = externalapi.OptString{}
	bad.Doc = externalapi.NewOptAutomationStepDoc(badHeadingDoc())
	invalid, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Steps: []externalapi.AutomationStep{bad}},
		externalapi.AutomationsUpdateParams{ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsUpdateUnprocessableEntity{}, invalid)
	stored, err := automations.Decode(env.DB.Automation.GetX(ctx, int64(res.ID)).Definition)
	require.NoError(t, err)
	assert.Contains(t, stored[0].Body, "Second")
}

func TestExternalEmailLintTakesADocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read")
	ctx := context.Background()

	ok, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{Doc: externalapi.NewOptLintEmailInputDoc(paragraphDoc("Hi {{ first_name }}"))})
	require.NoError(t, err)
	res, isResult := ok.(*externalapi.LintEmailResult)
	require.Truef(t, isResult, "got %T", ok)
	assert.Empty(t, res.Errors)

	bad, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{Doc: externalapi.NewOptLintEmailInputDoc(badHeadingDoc())})
	require.NoError(t, err)
	got, isResult := bad.(*externalapi.LintEmailResult)
	require.Truef(t, isResult, "got %T", bad)
	require.Len(t, got.Errors, 1)
	assert.Contains(t, got.Errors[0].Detail.Or(""), "level")

	both, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{
		Body: externalapi.NewOptString("<p>x</p>"), Doc: externalapi.NewOptLintEmailInputDoc(paragraphDoc("x")),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailLintLintUnprocessableEntity{}, both)

	neither, err := c.EmailLintLint(ctx, &externalapi.LintEmailInput{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.EmailLintLintUnprocessableEntity{}, neither)
}
