package site_test

import (
	"context"
	"net/http"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An id that is no public id of this Instance is refused (a 400 problem) before any handler runs,
// in a path and in a query: of the wrong form, or of the right form and not sealed under the
// Instance's id key (a forged or another Instance's id). The typed client cannot spell one, so
// each call carries a sentinel id whose public form is swapped on the wire.
func TestSiteMalformedIDIsABadRequest(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	for _, bad := range []string{"abc", "99999999999999999999", "1.5", "AAAAAAAAAAAAAAAAAAAAAA"} {
		t.Run(bad, func(t *testing.T) {
			c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.RewriteURL(testhelper.ID(123456789), bad))

			_, err := c.SiteContactsGet(ctx, siteapi.SiteContactsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 123456789})
			testhelper.SiteRefusal(t, err, http.StatusBadRequest, bad)

			_, err = c.SiteEventsList(ctx, siteapi.SiteEventsListParams{WorkspaceId: fixtures.AcmePublicID, ContactId: siteapi.NewOptEntityId(123456789)})
			testhelper.SiteRefusal(t, err, http.StatusBadRequest, "a malformed contactId query is refused: %s", bad)
		})
	}
}
