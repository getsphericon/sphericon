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

const hostileBody = `<p onclick="steal()">Hi {{ first_name }}</p><script>alert(1)</script><a href="javascript:alert(1)">x</a>`

// The body is sanitized where it is saved: a Template and a Broadcast keep the merge tags
// and lose script, event handlers and javascript: URLs (ADR 0040).
func TestSiteSanitizesBodiesOnSave(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	created, err := c.SiteTemplatesCreate(ctx, &siteapi.CreateTemplateInput{
		Name: "Hostile", Body: siteapi.NewOptString(hostileBody),
	}, siteapi.SiteTemplatesCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	tpl, ok := created.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, `<p>Hi {{ first_name }}</p>x`, tpl.Body)

	updated, err := c.SiteTemplatesUpdate(ctx, &siteapi.UpdateTemplateInput{
		Body: siteapi.NewOptString(`<img src="x" onerror="alert(1)">{{ first_name }}`),
	}, siteapi.SiteTemplatesUpdateParams{WorkspaceId: workspaceID, ID: tpl.ID})
	require.NoError(t, err)
	upd, ok := updated.(*siteapi.SiteEmailTemplateResource)
	require.Truef(t, ok, "got %T", updated)
	assert.NotContains(t, upd.Body, "onerror")
	assert.Contains(t, upd.Body, "{{ first_name }}")

	bc, err := c.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
		Name: "Hostile", Body: siteapi.NewOptString(hostileBody),
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	created2, ok := bc.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", bc)
	assert.Equal(t, `<p>Hi {{ first_name }}</p>x`, created2.Body)

	bu, err := c.SiteBroadcastsUpdate(ctx, &siteapi.UpdateBroadcastInput{
		Body: siteapi.NewOptString(hostileBody),
	}, siteapi.SiteBroadcastsUpdateParams{WorkspaceId: workspaceID, ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	updated2, ok := bu.(*siteapi.SiteBroadcastResource)
	require.Truef(t, ok, "got %T", bu)
	assert.Equal(t, `<p>Hi {{ first_name }}</p>x`, updated2.Body)
}
