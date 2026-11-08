package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func automationReport(t *testing.T, c *externalapi.Client, params externalapi.AutomationsReportParams) *externalapi.AutomationReport {
	t.Helper()
	report, err := c.AutomationsReport(context.Background(), params)
	require.NoError(t, err)
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

	// The Enrollment walks its two tag steps and completes.
	require.NoError(t, env.StepExecution.Step(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationRunTagOnEngagementAliceID, 0))
	env.RunQueuedSteps(t, env.StepExecution)
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

	_, foreign := env.ExternalScoped(t, "automations:read").AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationGlobexID)})
	_, denied := env.ExternalScoped(t, "contacts:read").AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})

	testhelper.ExternalRefusal(t, foreign, http.StatusNotFound)
	testhelper.ExternalRefusal(t, denied, http.StatusForbidden)
}
