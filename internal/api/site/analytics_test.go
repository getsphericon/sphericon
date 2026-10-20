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

// The analytics overview requires a valid JWT cookie like the rest of the site API.
func TestSiteAnalyticsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsOverview(context.Background(), siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

// A workspaceID the user does not own is a 404, not a data leak.
func TestSiteAnalyticsRequireOwnedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteAnalyticsOverview(context.Background(), siteapi.SiteAnalyticsOverviewParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, out)
}

// The handler only maps the analytics module's Overview (rules are tested in
// internal/analytics): the range param picks the window and the fields carry over.
func TestSiteAnalyticsOverviewMapsTheModule(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{
		WorkspaceId: fixtures.AcmePublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
	})
	require.NoError(t, err)
	ov, ok := res.(*siteapi.SiteAnalyticsOverview)
	require.Truef(t, ok, "got %T", res)

	assert.Len(t, ov.Timeseries, 7)
	assert.Equal(t, int32(11), ov.Email.SentCount)
	assert.Equal(t, int32(5), ov.Email.OpenedCount)
	assert.Equal(t, int32(2), ov.Email.ClickedCount)
	assert.Greater(t, ov.Contacts.Total, int32(0))
	assert.Greater(t, ov.Automations.Total, int32(0))

	res, err = c.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Len(t, res.(*siteapi.SiteAnalyticsOverview).Timeseries, 30, "30d is the default window")
}
