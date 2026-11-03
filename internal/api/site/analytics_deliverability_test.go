package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The rows are in fixtures/events.yml ("Sending-domain rates of the last 24 hours"): the Acme
// domains launch, lost and old have no other traffic, so what the fixture gives them is all they
// have.

func ratesOf(domains []siteapi.SiteSendingDomainRates, domain string) (siteapi.SiteSendingDomainRates, bool) {
	return lo.Find(domains, func(d siteapi.SiteSendingDomainRates) bool { return d.Domain == domain })
}

func overviewDeliverability(t *testing.T, env *testhelper.TestEnv) siteapi.SiteAnalyticsDeliverability {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAnalyticsOverview(context.Background(),
		siteapi.SiteAnalyticsOverviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	ov, ok := res.(*siteapi.SiteAnalyticsOverview)
	require.Truef(t, ok, "got %T", res)
	return ov.Deliverability
}

func deliverabilityReport(t *testing.T, env *testhelper.TestEnv, email string, rng siteapi.SiteAnalyticsRange) siteapi.SiteAnalyticsDeliverability {
	t.Helper()
	res, err := env.SiteActor(t, email).SiteAnalyticsDeliverability(context.Background(), siteapi.SiteAnalyticsDeliverabilityParams{
		WorkspaceId: fixtures.AcmePublicID,
		Range:       siteapi.NewOptSiteAnalyticsRange(rng),
	})
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAnalyticsDeliverability)
	require.Truef(t, ok, "%s got %T", email, res)
	return *report
}

// Without any license, the overview carries each domain's rates over the last 24 hours as the
// triples they are made of: launch.acme.com has 1 complaint on 5 delivered and 2 hard bounces on 7 sent.
func TestOverviewCarriesTheSendingDomainRatesOfTheLast24Hours(t *testing.T) {
	env := testhelper.Setup(t)

	deliverability := overviewDeliverability(t, env)
	rates, listed := ratesOf(deliverability.Domains, fixtures.SendingDomainLaunchDomain)

	assert.EqualValues(t, 24, deliverability.WindowHours)
	require.True(t, listed)
	assert.EqualValues(t, 1, rates.ComplaintRate.Numerator)
	assert.EqualValues(t, 5, rates.ComplaintRate.Denominator)
	assert.InDelta(t, 0.2, rates.ComplaintRate.Rate.Value, 1e-9)
	assert.EqualValues(t, 2, rates.BounceRate.Numerator)
	assert.EqualValues(t, 7, rates.BounceRate.Denominator)
	assert.InDelta(t, 2.0/7.0, rates.BounceRate.Rate.Value, 1e-9)
}

// There is no display floor: one send and one hard bounce are a Bounce rate of 1.0, and with
// nothing delivered the Complaint rate is undefined, never 0.
func TestOverviewShowsARateWithoutAFloorAndNullWithoutADenominator(t *testing.T) {
	env := testhelper.Setup(t)

	rates, listed := ratesOf(overviewDeliverability(t, env).Domains, fixtures.SendingDomainLostDomain)

	require.True(t, listed)
	assert.EqualValues(t, 1, rates.BounceRate.Denominator)
	assert.InDelta(t, 1.0, rates.BounceRate.Rate.Value, 1e-9)
	assert.True(t, rates.ComplaintRate.Rate.Null)
	assert.Zero(t, rates.ComplaintRate.Denominator)
}

// Mail sent 3 days ago is in the 7d report and not in the overview's 24-hour cut.
func TestMailOfThreeDaysAgoIsInTheReportAndNotInTheOverview(t *testing.T) {
	env := testhelper.Setup(t)

	inOverview, _ := ratesOf(overviewDeliverability(t, env).Domains, fixtures.SendingDomainArchivedDomain)
	report := deliverabilityReport(t, env, fixtures.OwnerJohnEmail, siteapi.SiteAnalyticsRange7d)
	inReport, listed := ratesOf(report.Domains, fixtures.SendingDomainArchivedDomain)

	assert.Zero(t, inOverview.BounceRate.Denominator, "the 4 sends of 3 days ago are outside the last 24 hours")
	require.True(t, listed)
	assert.EqualValues(t, 4, inReport.BounceRate.Denominator, "sent is the Bounce rate's denominator")
	assert.EqualValues(t, 7*24, report.WindowHours)
}

// A domain with no traffic in the window is not listed; an archived one that got a complaint in it is.
func TestReportListsOnlyDomainsWithTrafficArchivedOnesIncluded(t *testing.T) {
	env := testhelper.Setup(t)

	report := deliverabilityReport(t, env, fixtures.OwnerJohnEmail, siteapi.SiteAnalyticsRange7d)

	archived, listed := ratesOf(report.Domains, fixtures.SendingDomainArchivedDomain)
	require.True(t, listed, "the archived domain's complaint is in the window")
	assert.EqualValues(t, 1, archived.ComplaintRate.Numerator)
	_, news := ratesOf(report.Domains, "news.acme.com")
	assert.False(t, news, "no traffic in the window")
	_, mail := ratesOf(report.Domains, "mail.acme.com")
	assert.True(t, mail, "the fixture traffic of the last 7 days")
}

func TestEveryRoleReadsTheDeliverabilityReport(t *testing.T) {
	env := testhelper.Setup(t)

	for _, email := range []string{fixtures.OwnerJohnEmail, fixtures.AdminAliceEmail, fixtures.MemberMaryEmail} {
		report := deliverabilityReport(t, env, email, siteapi.SiteAnalyticsRange7d)
		assert.NotEmpty(t, report.Domains, email)
	}
}

func TestSiteAnalyticsDeliverabilityRequiresAuthAndAnOwnedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteAnalyticsDeliverability(context.Background(),
		siteapi.SiteAnalyticsDeliverabilityParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)

	out, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAnalyticsDeliverability(context.Background(),
		siteapi.SiteAnalyticsDeliverabilityParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAnalyticsDeliverabilityNotFound{}, out)
}
