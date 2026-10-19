package site_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func paragraphDoc(text string) map[string]jx.Raw {
	return map[string]jx.Raw{
		"type": jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"paragraph","content":[{"type":"text","text":` +
			strconv.Quote(text) + `}]}]`),
	}
}

func TestSiteTemplatesSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name:    "Authored",
		Subject: siteapi.NewOptString("Hi {{ first_name }}"),
		Doc:     siteapi.NewOptSiteCreateEmailTemplateInputDoc(paragraphDoc("Hello {{ first_name }}")),
	}, siteapi.SiteTemplatesCreateParams{Slug: slug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, res.Body, "Hello {{ first_name }}")
	assert.True(t, res.Doc.Set && !res.Doc.Null, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), res.EditorVersion.Value)

	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Equal(t, res.Body, row.Body)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(row.Doc))
	require.NotNil(t, row.EditorVersion)
	assert.Equal(t, emaildoc.EditorVersion(), *row.EditorVersion)
	assert.Equal(t, emailrender.SanitizeHTML(row.Body), row.Body, "the stored body survives the sanitizer")

	// Saving another document rebuilds the body.
	updated, err := c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{
		Doc: siteapi.NewOptSiteUpdateEmailTemplateInputDoc(paragraphDoc("Second")),
	}, siteapi.SiteTemplatesUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Contains(t, upd.Body, "Second")
	assert.NotContains(t, upd.Body, "Hello")

	// A subject-only write leaves the document alone.
	_, err = c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{Subject: siteapi.NewOptString("New subject")},
		siteapi.SiteTemplatesUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	assert.NotEmpty(t, env.DB.EmailTemplate.GetX(ctx, int64(res.ID)).Doc)

	// A body-only write clears the document and its editor version.
	cleared, err := c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{Body: siteapi.NewOptString("<p>Plain</p>")},
		siteapi.SiteTemplatesUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	clr, ok := cleared.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", cleared)
	assert.False(t, clr.Doc.Set && !clr.Doc.Null, "the response carries no document")
	row = env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
	assert.Equal(t, "<p>Plain</p>", row.Body)
}

func TestSiteTemplatesWithoutADocumentStayHTMLOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Html", Body: siteapi.NewOptString("<p>Hi</p>"),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)

	row := env.DB.EmailTemplate.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
}

func TestSiteTemplatesRefuseADocumentThatCannotBeRendered(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug
	before := env.DB.EmailTemplate.Query().CountX(ctx)
	welcome := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	badLevel := map[string]jx.Raw{
		"type":    jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`),
	}
	created, err := c.SiteTemplatesCreate(ctx, &siteapi.SiteCreateEmailTemplateInput{
		Name: "Bad", Doc: siteapi.NewOptSiteCreateEmailTemplateInputDoc(badLevel),
	}, siteapi.SiteTemplatesCreateParams{Slug: slug})
	require.NoError(t, err)
	refused, ok := created.(*siteapi.SiteTemplatesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, refused.Detail.Value, "level")
	assert.Equal(t, before, env.DB.EmailTemplate.Query().CountX(ctx), "nothing is stored")

	notADoc := map[string]jx.Raw{"type": jx.Raw(`"nope"`)}
	updated, err := c.SiteTemplatesUpdate(ctx, &siteapi.SiteUpdateEmailTemplateInput{
		Name: siteapi.NewOptString("Renamed"), Doc: siteapi.NewOptSiteUpdateEmailTemplateInputDoc(notADoc),
	}, siteapi.SiteTemplatesUpdateParams{Slug: slug, ID: idStr(fixtures.TemplateWelcomeID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesUpdateUnprocessableEntity{}, updated)
	after := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	assert.Equal(t, welcome.Name, after.Name, "nothing is stored")
	assert.Equal(t, welcome.Body, after.Body)
}

func TestSiteTemplatesRefuseABodyAndADocumentTogether(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteTemplatesCreate(context.Background(), &siteapi.SiteCreateEmailTemplateInput{
		Name: "Both", Body: siteapi.NewOptString("<p>x</p>"),
		Doc: siteapi.NewOptSiteCreateEmailTemplateInputDoc(paragraphDoc("x")),
	}, siteapi.SiteTemplatesCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTemplatesCreateUnprocessableEntity{}, created)
}
