package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func automationReport(t *testing.T, c *externalapi.Client, params externalapi.AutomationsReportParams) *externalapi.AutomationReport {
	t.Helper()
	res, err := c.AutomationsReport(context.Background(), params)
	require.NoError(t, err)
	report, ok := res.(*externalapi.AutomationReport)
	require.Truef(t, ok, "got %T", res)
	return report
}

// The results are read from the Enrollments (#753): one that ends today counts in the current
// period under its outcome, and the period before does not move.
func TestExternalAutomationReportCountsAnEnrollmentThatEndsTodayInThePeriod(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read")
	params := externalapi.AutomationsReportParams{
		ID:     entityIDString(fixtures.AutomationTagOnEngagementID),
		Period: externalapi.NewOptAutomationsReportPeriod(externalapi.AutomationsReportPeriod14d),
	}
	before := automationReport(t, c, params)

	require.NoError(t, automations.Complete(context.Background(), env.DB.Scoped(fixtures.AcmeID), env.Publisher(), fixtures.AutomationRunTagOnEngagementAliceID, time.Now()))
	after := automationReport(t, c, params)

	assert.Equal(t, externalapi.AutomationReportPeriod14d, after.Results.Period)
	assert.Len(t, after.Results.Series, 14)
	assert.Equal(t, before.Results.Current.Completed+1, after.Results.Current.Completed)
	assert.Equal(t, before.Results.Previous, after.Results.Previous)
}

func TestExternalAutomationReportReadsThirtyDaysByDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read")

	report := automationReport(t, c, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})

	assert.Equal(t, externalapi.AutomationReportPeriod30d, report.Results.Period)
	assert.Len(t, report.Results.Series, 30)
}

func TestExternalAutomationReportNeedsTheReadScopeAndTheTokensWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	foreign, err := env.ExternalScoped(t, "automations:read").AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationGlobexID)})
	require.NoError(t, err)
	denied, err := env.ExternalScoped(t, "contacts:read").AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.AutomationsReportNotFound{}, foreign)
	assert.IsType(t, &externalapi.AutomationsReportForbidden{}, denied)
}
