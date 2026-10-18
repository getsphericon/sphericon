package site_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A Suppression id of another Workspace is invisible: deleting it is a 404 and the
// row survives.
func TestSiteSuppressionsDeleteForeignIDIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	out, err := c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{
		Slug: fixtures.AcmeSlug, ID: siteapi.EntityId(int64(fixtures.SuppressionGlobexID)),
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSuppressionsDeleteNotFound{}, out)

	exists, err := env.DB.Suppression.Query().Where(suppression.ID(fixtures.SuppressionGlobexID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "the other Workspace's row is untouched")
}
