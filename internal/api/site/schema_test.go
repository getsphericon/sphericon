package site_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Only a development build runs migrations from the error page; a server built without them
// answers 404 to anyone.
func TestSiteSchemaApplyMigrationsIsNotFoundWhereTheServerDoesNotOfferIt(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteAnonymous(t).SiteSchemaApplyMigrations(t.Context())

	require.NoError(t, err)
	problem, ok := res.(*siteapi.ProblemDetails)
	require.True(t, ok, "expected a problem, got %T", res)
	assert.Equal(t, int32(404), problem.Status.Value)
}
