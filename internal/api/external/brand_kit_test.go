package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalBrandKitGetReadsTheWorkspacesBrandKit(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	denied, err := env.ExternalScoped(t, "contacts:read").BrandKitGet(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.BrandKitGetForbidden{}, denied, "templates:read is required")

	res, err := env.ExternalScoped(t, "templates:read").BrandKitGet(ctx)
	require.NoError(t, err)
	kit, ok := res.(*externalapi.BrandKitResource)
	require.Truef(t, ok, "got %T", res)
	// Acme's saved Brand kit (fixtures/brand_kits.yml), not another Workspace's.
	assert.Equal(t, "#1a73e8", kit.AccentColor)
	assert.True(t, kit.UpdatedAt.Set && !kit.UpdatedAt.Null, "a saved Brand kit carries when it was saved")
}
