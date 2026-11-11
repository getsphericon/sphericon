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

func TestSiteBroadcastsSaveADocumentWithItsBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Authored",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hi {{ first_name }}"), Doc: siteapi.NewOptEditorDocument(paragraphDoc("Hello {{ first_name }}"))}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	assert.Contains(t, testhelper.SiteEmailOf(res).Body, "Hello {{ first_name }}")
	assert.True(t, testhelper.SiteEmailOf(res).Doc.Set && !testhelper.SiteEmailOf(res).Doc.Null, "the response carries the document")
	assert.Equal(t, emaildoc.EditorVersion(), testhelper.SiteEmailOf(res).EditorVersion.Value)

	row := env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Equal(t, testhelper.SiteEmailOf(res).Body, *row.Body)
	assert.JSONEq(t, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello {{ first_name }}"}]}]}`, string(row.Doc))
	require.NotNil(t, row.EditorVersion)

	// Saving another document rebuilds the body.
	updated, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Doc: siteapi.NewOptEditorDocument(paragraphDoc("Second"))}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	upd := updated
	assert.Contains(t, testhelper.SiteEmailOf(upd).Body, "Second")
	assert.NotContains(t, testhelper.SiteEmailOf(upd).Body, "Hello")

	// A name-only write leaves the document alone.
	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Name: siteapi.NewOptName("Renamed")},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	assert.NotEmpty(t, env.DB.Broadcast.GetX(ctx, int64(res.ID)).Doc)

	// A body-only write clears the document and its editor version.
	cleared, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString("<p>Plain</p>")})},
		siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)
	clr := cleared
	assert.False(t, testhelper.SiteEmailOf(clr).Doc.Set && !testhelper.SiteEmailOf(clr).Doc.Null, "the response carries no document")
	row = env.DB.Broadcast.GetX(ctx, int64(res.ID))
	assert.Empty(t, row.Doc)
	assert.Nil(t, row.EditorVersion)
	assert.Equal(t, "<p>Plain</p>", *row.Body)
}

func TestSiteBroadcastsRefuseADocumentThatCannotBeRendered(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	before := env.DB.Broadcast.Query().CountX(ctx)
	draft := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)

	badLevel := map[string]jx.Raw{
		"type":    jx.Raw(`"doc"`),
		"content": jx.Raw(`[{"type":"heading","attrs":{"level":9},"content":[{"type":"text","text":"x"}]}]`),
	}
	_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Bad",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Doc: siteapi.NewOptEditorDocument(badLevel)}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Value, "level")
	assert.Equal(t, before, env.DB.Broadcast.Query().CountX(ctx), "nothing is stored")

	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Name:    siteapi.NewOptName("Renamed"),
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Doc: siteapi.NewOptEditorDocument(map[string]jx.Raw{"type": jx.Raw(`"nope"`)})}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: idStr(fixtures.BroadcastDraftID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	after := env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID)
	assert.Equal(t, draft.Name, after.Name, "nothing is stored")
	assert.Equal(t, draft.Body, after.Body)
}

func TestSiteBroadcastsRefuseABodyAndADocumentTogether(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	_, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "Both",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptEditorDocument(paragraphDoc("x"))}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString("<p>x</p>"), Doc: siteapi.NewOptEditorDocument(paragraphDoc("x"))}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestSiteBroadcastsWithoutADocumentStayHTMLOnly(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	got, err := c.SiteBroadcastsGet(ctx, siteapi.SiteBroadcastsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
	require.NoError(t, err)
	res := got
	assert.False(t, testhelper.SiteEmailOf(res).Doc.Set && !testhelper.SiteEmailOf(res).Doc.Null)
	assert.Empty(t, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Doc)
}

// A Broadcast written in the editor is previewed from the body the server built: the test send
// delivers it with sample merge data.
func TestSiteBroadcastsTestSendDeliversTheMessageBuiltFromADocument(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteBroadcastsTestSend(context.Background(), &siteapi.TestSendBroadcastInput{Email: siteapi.NewOptEmailAddress("preview@example.com")},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastAuthoredID)})
	require.NoError(t, err)

	msgs := env.CustomerMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "preview@example.com", msgs[0].To)
	assert.Contains(t, msgs[0].HTML, "What is new")
	assert.Contains(t, msgs[0].HTML, "Browse courses")
	assert.NotContains(t, msgs[0].HTML, "<script")
}
