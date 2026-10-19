package site_test

import (
	"context"
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

func TestSiteBroadcastsSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name:    "Authored",
		Subject: siteapi.NewOptString("Hi {{ first_name }}"),
		Doc:     siteapi.NewOptSiteCreateBroadcastInputDoc(paragraphDoc("Hello {{ first_name }}")),
	}, siteapi.SiteBroadcastsCreateParams{Slug: slug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, res.Body, "Hello {{ first_name }}")
	assert.True(t, res.Doc.Set && !res.Doc.Null, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), res.EditorVersion.Value)

	row := env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Equal(t, res.Body, row.Body)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(row.Doc))
	require.NotNil(t, row.EditorVersion)
	assert.Equal(t, emailrender.SanitizeHTML(row.Body), row.Body, "the stored body survives the sanitizer")

	// Saving another document rebuilds the body.
	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		Doc: siteapi.NewOptSiteUpdateBroadcastInputDoc(paragraphDoc("Second")),
	}, siteapi.SiteBroadcastsUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Contains(t, upd.Body, "Second")
	assert.NotContains(t, upd.Body, "Hello")

	// A name-only write leaves the document alone.
	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{Name: siteapi.NewOptString("Renamed")},
		siteapi.SiteBroadcastsUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	assert.NotEmpty(t, env.DB.Broadcast.GetX(ctx, int64(res.ID)).Doc)

	// A body-only write clears the document and its editor version.
	cleared, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{Body: siteapi.NewOptString("<p>Plain</p>")},
		siteapi.SiteBroadcastsUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	clr, ok := cleared.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", cleared)
	assert.False(t, clr.Doc.Set && !clr.Doc.Null, "the response carries no document")
	row = env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
	assert.Equal(t, "<p>Plain</p>", row.Body)
}

func TestSiteBroadcastsRefuseADocumentThatCannotBeRendered(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug
	before := env.DB.Broadcast.Query().CountX(ctx)
	draft := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	badLevel := map[string]jx.Raw{
		"type":    jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`),
	}
	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "Bad", Doc: siteapi.NewOptSiteCreateBroadcastInputDoc(badLevel),
	}, siteapi.SiteBroadcastsCreateParams{Slug: slug})
	require.NoError(t, err)
	refused, ok := created.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", created)
	assert.Contains(t, refused.Detail.Value, "level")
	assert.Equal(t, before, env.DB.Broadcast.Query().CountX(ctx), "nothing is stored")

	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		Name: siteapi.NewOptString("Renamed"),
		Doc:  siteapi.NewOptSiteUpdateBroadcastInputDoc(map[string]jx.Raw{"type": jx.Raw(`"nope"`)}),
	}, siteapi.SiteBroadcastsUpdateParams{Slug: slug, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, updated)
	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, draft.Name, after.Name, "nothing is stored")
	assert.Equal(t, draft.Body, after.Body)
}

func TestSiteBroadcastsRefuseABodyAndADocumentTogether(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.SiteCreateBroadcastInput{
		Name: "Both", Body: siteapi.NewOptString("<p>x</p>"),
		Doc: siteapi.NewOptSiteCreateBroadcastInputDoc(paragraphDoc("x")),
	}, siteapi.SiteBroadcastsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsCreateUnprocessableEntity{}, created)

	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{
		Body: siteapi.NewOptString("<p>x</p>"),
		Doc:  siteapi.NewOptSiteUpdateBroadcastInputDoc(paragraphDoc("x")),
	}, siteapi.SiteBroadcastsUpdateParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteBroadcastsUpdateUnprocessableEntity{}, updated)
}

func TestSiteBroadcastsWithoutADocumentStayHTMLOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)
	res, ok := got.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", got)
	assert.False(t, res.Doc.Set && !res.Doc.Null)
	assert.Empty(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Doc)
}

// A Broadcast written in the editor is previewed from the body the server built: the test send
// delivers it with sample merge data, sanitized.
func TestSiteBroadcastsTestSendDeliversTheMessageBuiltFromADocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteBroadcastsTestSend(context.Background(), &siteapi.SiteTestSendBroadcastInput{Email: "preview@example.com"},
		siteapi.SiteBroadcastsTestSendParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.BroadcastAuthoredID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteBroadcastsTestSendNoContent{}, res)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "preview@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].HTML, "What is new")
	assert.Contains(t, msgs[0].HTML, "Browse courses")
	assert.NotContains(t, msgs[0].HTML, "<script")
}
