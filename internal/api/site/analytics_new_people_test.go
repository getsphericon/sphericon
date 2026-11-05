package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteAnalyticsNewPeopleRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsNewPeople(context.Background(), siteapi.SiteAnalyticsNewPeopleParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteAnalyticsNewPeopleIsRefusedForAForeignWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteAnalyticsNewPeople(context.Background(), siteapi.SiteAnalyticsNewPeopleParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestEveryRoleReadsTheNewPeopleReport(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	for _, email := range []string{fixtures.OwnerJohnEmail, fixtures.AdminAliceEmail, fixtures.MemberMaryEmail} {
		c := env.SiteActor(t, email)
		res, err := c.SiteAnalyticsNewPeople(ctx, siteapi.SiteAnalyticsNewPeopleParams{
			WorkspaceId: fixtures.AcmePublicID,
			Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
		})
		require.NoError(t, err, email)
		report := res
		assert.Len(t, report.Timeseries, 7, email)
	}
}
