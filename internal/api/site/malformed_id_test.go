package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An id that is not an int64 is refused by the contract (a 400 problem) before any
// handler runs, in a path and in a query. The typed client cannot spell one, so each
// call carries a sentinel id that is swapped on the wire.
func TestSiteMalformedIDIsABadRequest(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	for _, bad := range []string{"abc", "99999999999999999999", "1.5"} {
		c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.RewriteURL("123456789", bad))

		inPath, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{Slug: fixtures.AcmeSlug, ID: 123456789})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteContactsGetBadRequest{}, inPath, bad)

		_, err = c.SiteEventsList(ctx, siteapi.SiteEventsListParams{Slug: fixtures.AcmeSlug, ContactId: siteapi.NewOptEntityId(123456789)})
		assert.Error(t, err, "a malformed contactId query is refused: %s", bad)
	}
}
