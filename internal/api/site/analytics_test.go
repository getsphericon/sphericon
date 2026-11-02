package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Events report needs a session, is refused for a foreign Workspace and is open to every
// role (its rules are tested in internal/analytics).
func TestSiteAnalyticsEventsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteAnalyticsEvents(context.Background(), siteapi.SiteAnalyticsEventsParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteAnalyticsEventsRequireOwnedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	out, err := c.SiteAnalyticsEvents(context.Background(), siteapi.SiteAnalyticsEventsParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAnalyticsEventsNotFound{}, out)
}

func TestSiteAnalyticsEventsAreVisibleToEveryRole(t *testing.T) {
	env := testhelper.Setup(t)
	for _, email := range []string{fixtures.OwnerJohnEmail, fixtures.AdminAliceEmail, fixtures.MemberMaryEmail} {
		c := env.SiteActor(t, email)
		res, err := c.SiteAnalyticsEvents(context.Background(), siteapi.SiteAnalyticsEventsParams{
			WorkspaceId: fixtures.AcmePublicID,
			Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
		})
		require.NoError(t, err, email)
		report, ok := res.(*siteapi.SiteAnalyticsEvents)
		require.Truef(t, ok, "%s: got %T", email, res)

		view, found := lo.Find(report.Actions, func(a siteapi.SiteAnalyticsEventAction) bool { return a.Action == "report_view" })
		require.True(t, found, email)
		assert.Equal(t, int32(4), view.Count, email)
		assert.Equal(t, int32(2), view.Contacts, email)
	}
}

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
	assert.IsType(t, &siteapi.SiteAnalyticsOverviewNotFound{}, out)
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
	assert.Equal(t, int32(11), ov.Current.Email.SentCount)
	assert.Equal(t, int32(5), ov.Current.Email.OpenedCount)
	assert.Equal(t, int32(2), ov.Current.Email.ClickedCount)
	assert.Positive(t, ov.Contacts.Total)
	assert.Positive(t, ov.Automations.Total)

	res, err = c.SiteAnalyticsOverview(ctx, siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Len(t, res.(*siteapi.SiteAnalyticsOverview).Timeseries, 7, "7d is the default window")
}

// Read on 14 March 2026 over 7 days, Gotham's periods are cut in New York: 8-14 March and 1-7
// March. Its Broadcast send of 23:59 EST on 7 March, 8 March in UTC, is in the previous period; the
// three sends after midnight are in the current one (outbound_messages.yml).
func TestSiteAnalyticsOverviewHasThePeriodBeforeTheRangeInTheWorkspaceTimeZone(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC) }))
	c := env.SiteActor(t, fixtures.GothamOwnerGusEmail)

	res, err := c.SiteAnalyticsOverview(t.Context(), siteapi.SiteAnalyticsOverviewParams{
		WorkspaceId: fixtures.GothamPublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(siteapi.SiteAnalyticsRange7d),
	})
	require.NoError(t, err)
	ov, ok := res.(*siteapi.SiteAnalyticsOverview)
	require.Truef(t, ok, "got %T", res)

	assert.Equal(t, "2026-03-08", ov.Current.From)
	assert.Equal(t, "2026-03-14", ov.Current.To)
	assert.Equal(t, "2026-03-01", ov.Previous.From)
	assert.Equal(t, "2026-03-07", ov.Previous.To)
	assert.Equal(t, siteapi.SiteAnalyticsSending{Broadcast: 3}, ov.Current.Sending)
	assert.Equal(t, siteapi.SiteAnalyticsSending{Broadcast: 1}, ov.Previous.Sending)
	assert.Equal(t, int32(1), ov.Previous.Email.SentCount)
	assert.Equal(t, ov.Current.From, ov.Timeseries[0].Date, "the series is the current period's")
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
