package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Only a development build runs migrations from the error page; a server built without them
// answers 404 to anyone.
func TestSiteSchemaApplyMigrationsIsNotFoundWhereTheServerDoesNotOfferIt(t *testing.T) {
	env := testhelper.Setup(t)

	err := env.SiteAnonymous(t).SiteSchemaApplyMigrations(t.Context())

	problem := testhelper.SiteRefusal(t, err, http.StatusNotFound)
	assert.Equal(t, int32(404), problem.Status.Value)
}
