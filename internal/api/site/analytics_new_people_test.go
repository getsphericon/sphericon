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

func TestSiteAnalyticsNewPeopleRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsNewPeople(context.Background(), siteapi.SiteAnalyticsNewPeopleParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteAnalyticsNewPeopleIsRefusedForAForeignWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteAnalyticsNewPeople(context.Background(), siteapi.SiteAnalyticsNewPeopleParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, out)
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
		report, ok := res.(*siteapi.SiteNewPeopleReport)
		require.Truef(t, ok, "%s got %T", email, res)
		assert.Len(t, report.Timeseries, 7, email)
	}
}
