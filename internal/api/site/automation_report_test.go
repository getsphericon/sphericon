package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func automationResults(t *testing.T, c *siteapi.Client, params siteapi.SiteAutomationReportsGetParams) siteapi.SiteAutomationResults {
	t.Helper()
	res, err := c.SiteAutomationReportsGet(t.Context(), params)
	require.NoError(t, err)
	report, ok := res.(*siteapi.SiteAutomationReport)
	require.Truef(t, ok, "got %T", res)
	return report.Results
}

// The results are read from the Enrollments (#753): one that ends today counts in the current
// period under its outcome, and the period before does not move.
func TestTheAutomationReportCountsAnEnrollmentThatEndsTodayInThePeriod(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	params := siteapi.SiteAutomationReportsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: idStr(fixtures.AutomationTagOnEngagementID),
		Period:       siteapi.NewOptSiteAutomationReportsGetPeriod(siteapi.SiteAutomationReportsGetPeriod7d),
	}
	before := automationResults(t, c, params)

	exited, err := automations.Exit(context.Background(), env.DB.Scoped(fixtures.AcmeID), env.Publisher(), fixtures.AutomationTagOnEngagementID, fixtures.ContactAliceID, time.Now())
	require.NoError(t, err)
	require.True(t, exited)
	after := automationResults(t, c, params)

	assert.Equal(t, siteapi.SiteAutomationReportPeriod7d, after.Period)
	assert.Len(t, after.Series, 7)
	assert.Equal(t, before.Current.Exited+1, after.Current.Exited)
	assert.Equal(t, before.Previous, after.Previous)
}

func TestTheAutomationReportReadsThirtyDaysByDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	results := automationResults(t, c, siteapi.SiteAutomationReportsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: idStr(fixtures.AutomationWelcomeSeriesID),
	})

	assert.Equal(t, siteapi.SiteAutomationReportPeriod30d, results.Period)
	assert.Len(t, results.Series, 30)
}
