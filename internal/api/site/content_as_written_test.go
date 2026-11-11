package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// writtenBody holds what a mail client may drop (a handler, a form, an at-rule): the author's
// markup all the same.
const writtenBody = `<p class="lead" onclick="track()">Hi {{ first_name }}</p>` +
	`<form action="https://example.com/survey"><input name="answer"></form>` +
	`<style>@keyframes pulse{from{opacity:0}}</style>`

// A body is stored as it was written: a Template and a Broadcast keep it byte for byte (ADR 0118).
func TestSiteStoresBodiesAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "As written", Body: siteapi.NewOptString(writtenBody),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.Equal(t, writtenBody, created.Body)

	updated, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{
		Body: siteapi.NewOptString(`<img src="https://example.com/a.png" onerror="fallback()">{{ first_name }}`),
	}, siteapi.SiteTemplatesUpdateParams{WorkspaceId: workspaceID, ID: created.ID})
	require.NoError(t, err)
	assert.Equal(t, `<img src="https://example.com/a.png" onerror="fallback()">{{ first_name }}`, updated.Body)

	bc, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name:    "As written",
		Content: testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString(writtenBody)}),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.Equal(t, writtenBody, testhelper.SiteEmailOf(bc).Body)

	bu, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Content: testhelper.SiteEmailEdit(siteapi.EmailBroadcastContentInput{Body: siteapi.NewOptString(writtenBody)}),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	assert.Equal(t, writtenBody, testhelper.SiteEmailOf(bu).Body)
}
