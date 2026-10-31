package analytics_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/analytics"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const (
	pricing = "https://codebasics.dev/pricing"
	docs    = "https://codebasics.dev/docs"
)

func clicksByLink(t *testing.T, w analytics.Window) []analytics.LinkClicks {
	t.Helper()
	env := testhelper.Setup(t)
	links, err := analytics.New().ClicksByLink(context.Background(), env.DB.Scoped(fixtures.AcmeID), w)
	require.NoError(t, err)
	return links
}

// Fixtures (events.yml): Broadcast 200 carries pricing twice (two days ago) and docs once
// (twenty days ago), Automation 102 carries pricing once, and one machine click on pricing.
func TestClicksByLinkSplitsEachLinkByTheBroadcastOrAutomationThatCarriedIt(t *testing.T) {
	links := clicksByLink(t, analytics.Window30Days)

	require.Len(t, links, 2)
	assert.Equal(t, pricing, links[0].URL)
	assert.Equal(t, 3, links[0].Clicks, "the machine click is left out")
	assert.Equal(t, []analytics.OwnerClicks{
		{Kind: analytics.OwnerBroadcast, ID: fixtures.BroadcastSentID, Name: "Python crash course is here", Clicks: 2},
		{Kind: analytics.OwnerAutomation, ID: fixtures.AutomationWinBackID, Name: "Win-back lapsed learners", Clicks: 1},
	}, links[0].Owners)
	assert.Equal(t, docs, links[1].URL)
	assert.Equal(t, 1, links[1].Clicks)
}

func TestClicksByLinkCountsOnlyTheWindow(t *testing.T) {
	links := clicksByLink(t, analytics.Window7Days)

	require.Len(t, links, 1)
	assert.Equal(t, pricing, links[0].URL)
}

func TestClicksByLinkEqualTheBroadcastAndAutomationReportsSummed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	links, err := analytics.New().ClicksByLink(ctx, s, analytics.Window90Days)
	require.NoError(t, err)
	got := map[string]int{}
	for _, l := range links {
		got[l.URL] = l.Clicks
	}

	want := map[string]int{}
	perBroadcast, err := broadcasts.New(env.Bus, testhelper.Renderer(), env.OutboundModule()).LinkClicksOf(ctx, s, fixtures.BroadcastSentID)
	require.NoError(t, err)
	for _, l := range perBroadcast {
		want[l.URL] += l.Clicks
	}
	perAutomation, err := automations.New(nil, env.Bus, testhelper.Renderer(), env.OutboundModule()).ReportOf(ctx, s, fixtures.AutomationWinBackID, automations.ReportPeriod30Days)
	require.NoError(t, err)
	for _, l := range perAutomation.Links {
		want[l.URL] += l.Clicks
	}
	assert.Equal(t, want, got)
}

func TestClicksByLinkIsEmptyForAWorkspaceWithoutClicks(t *testing.T) {
	env := testhelper.Setup(t)
	links, err := analytics.New().ClicksByLink(context.Background(), env.DB.Scoped(fixtures.GlobexID), analytics.Window90Days)
	require.NoError(t, err)
	assert.Empty(t, links)
}
