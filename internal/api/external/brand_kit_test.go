package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalBrandKitGetReadsTheWorkspacesBrandKit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := env.ExternalScoped(t, apitokens.ScopeContactsRead).BrandKitGet(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden, "templates:read is required")

	kit, err := env.ExternalScoped(t, apitokens.ScopeTemplatesRead).BrandKitGet(ctx)
	require.NoError(t, err)
	// Acme's saved Brand kit (fixtures/brand_kits.yml), not another Workspace's.
	assert.Equal(t, "#1a73e8", kit.AccentColor)
	assert.True(t, kit.UpdatedAt.Set && !kit.UpdatedAt.Null, "a saved Brand kit carries when it was saved")
}
