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
	assert.Positive(t, ov.Contacts.Total)
	assert.Positive(t, ov.Automations.Total)

	res, err = c.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Len(t, res.(*siteapi.SiteAnalyticsOverview).Timeseries, 30, "30d is the default window")
}

func TestSubscribedPeopleRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsSubscribedPeople(context.Background(), siteapi.SiteAnalyticsSubscribedPeopleParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

// The handler maps the module's report (rules are tested in internal/analytics).
func TestSubscribedPeopleMapsTheModule(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteAnalyticsSubscribedPeople(ctx, siteapi.SiteAnalyticsSubscribedPeopleParams{
		WorkspaceId: fixtures.AcmePublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
	})
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAnalyticsSubscribedPeople)
	require.Truef(t, ok, "got %T", res)

	assert.Len(t, report.Series, 7)
	assert.Positive(t, report.Current)
	for _, p := range report.Series {
		assert.Equal(t, p.Subscribed-p.Unsubscribed, p.Net)
	}
}

func TestSiteAnalyticsClicksByLinkRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsClicksByLink(context.Background(), siteapi.SiteAnalyticsClicksByLinkParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

// Every Membership role reads the report; the handler maps the module's links with their
// owners and the configured Event retention in days (rules are tested in internal/analytics).
func TestSiteAnalyticsClicksByLinkIsReadByAMemberAndCarriesTheRetention(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := c.SiteAnalyticsClicksByLink(context.Background(), siteapi.SiteAnalyticsClicksByLinkParams{
		WorkspaceId: fixtures.AcmePublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
	})
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAnalyticsClicksByLink)
	require.Truef(t, ok, "got %T", res)

	require.Len(t, report.Links, 1)
	assert.Equal(t, "https://codebasics.dev/pricing", report.Links[0].URL)
	assert.Equal(t, int32(3), report.Links[0].Clicks)
	assert.Equal(t, []siteapi.SiteAnalyticsLinkOwner{
		{Kind: siteapi.SiteAnalyticsOwnerKindBroadcast, ID: siteapi.EntityId(fixtures.BroadcastSentID), Name: "Python crash course is here", Clicks: 2},
		{Kind: siteapi.SiteAnalyticsOwnerKindAutomation, ID: siteapi.EntityId(fixtures.AutomationWinBackID), Name: "Win-back lapsed learners", Clicks: 1},
	}, report.Links[0].Owners)
	assert.Equal(t, int32(400), report.EventsRetentionDays, "the default SPHERICON_EVENTS_RETENTION_DAYS")
}

// The Opens by hour report is open to every role, refused without a session, and reads the
// Workspace time zone (cells are tested in internal/analytics).
func TestSiteAnalyticsOpensByHourRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsOpensByHour(context.Background(), siteapi.SiteAnalyticsOpensByHourParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteAnalyticsOpensByHourMapsTheModule(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteAnalyticsOpensByHour(context.Background(), siteapi.SiteAnalyticsOpensByHourParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAnalyticsOpensByHour)
	require.Truef(t, ok, "got %T", res)

	assert.Len(t, report.Cells, 7*24)
	var sum int32
	for _, cell := range report.Cells {
		sum += cell.Opens
	}
	assert.Equal(t, report.Total, sum)
}
