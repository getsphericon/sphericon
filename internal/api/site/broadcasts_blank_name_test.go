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

// A blank Broadcast name, empty or whitespace only, is the client's to fix: 422 with the same
// message the external API gives. The rule itself is tested at the broadcasts module.
func TestSiteBroadcastsRefuseABlankNameWith422(t *testing.T) {
	for _, name := range []string{"", "   "} {
		t.Run("create "+name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.SiteActor(t, fixtures.OwnerJohnEmail)

			res, err := c.SiteBroadcastsCreate(context.Background(), &siteapi.SiteCreateBroadcastInput{Name: name},
				siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
			require.NoError(t, err)
			refused, ok := res.(*siteapi.SiteBroadcastsCreateUnprocessableEntity)
			require.Truef(t, ok, "got %T", res)
			assert.Equal(t, "name must not be empty", refused.Detail.Value)
		})
		t.Run("update "+name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.SiteActor(t, fixtures.OwnerJohnEmail)
			ctx := context.Background()

			res, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{Name: siteapi.NewOptString(name)},
				siteapi.SiteBroadcastsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.BroadcastDraftID)})
			require.NoError(t, err)
			refused, ok := res.(*siteapi.SiteBroadcastsUpdateUnprocessableEntity)
			require.Truef(t, ok, "got %T", res)
			assert.Equal(t, "name must not be empty", refused.Detail.Value)
			assert.Equal(t, fixtures.BroadcastDraftName, env.DB.Broadcast.GetX(ctx, fixtures.BroadcastDraftID).Name)
		})
	}
}
