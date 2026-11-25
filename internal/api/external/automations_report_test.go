package external_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
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
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)
	params := externalapi.AutomationsReportParams{
		ID:     entityIDString(fixtures.AutomationTagOnEngagementID),
		Period: externalapi.NewOptAutomationsReportPeriod(externalapi.AutomationsReportPeriod14d),
	}
	before := automationReport(t, c, params)

	// The Enrollment walks its two tag steps and completes.
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).Step(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationRunTagOnEngagementAliceID, env.StepIDAt(t, fixtures.AutomationRunTagOnEngagementAliceID, 0)))
	env.RunQueuedSteps(t, testhelper.Module[*automations.StepExecution](env))
	after := automationReport(t, c, params)

	assert.Equal(t, externalapi.AutomationReportPeriod14d, after.Results.Period)
	assert.Len(t, after.Results.Series, 14)
	assert.Equal(t, before.Results.Current.Completed+1, after.Results.Current.Completed)
	assert.Equal(t, before.Results.Previous, after.Results.Previous)
}

func TestExternalAutomationReportReadsThirtyDaysByDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	report := automationReport(t, c, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})

	assert.Equal(t, externalapi.AutomationReportPeriod30d, report.Results.Period)
	assert.Len(t, report.Results.Series, 30)
}

// The report shows the skipped entries of the period by reason (ADR 0127): Liam's `once` a day ago
// and Emma's `active_enrollment` three days ago; the one 500 days old is in no period.
func TestExternalAutomationReportShowsTheSkippedEntriesByReason(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	report := automationReport(t, c, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})

	assert.Equal(t, externalapi.AutomationSkippedCounts{ActiveEnrollment: 1, Once: 1}, report.Results.Current.Skipped)
	assert.Equal(t, externalapi.AutomationSkippedCounts{}, report.Results.Previous.Skipped)
}

func TestExternalAutomationReportNeedsTheReadScopeAndTheTokensWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, foreign := env.ExternalScoped(t, apitokens.ScopeAutomationsRead).AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationGlobexID)})
	_, denied := env.ExternalScoped(t, apitokens.ScopeContactsRead).AutomationsReport(ctx, externalapi.AutomationsReportParams{ID: entityIDString(fixtures.AutomationWelcomeSeriesID)})

	testhelper.ExternalRefusal(t, foreign, http.StatusNotFound)
	testhelper.ExternalRefusal(t, denied, http.StatusForbidden)
}

// An integration reads each fork path's Enrollments and goals in the report (ADR 0124): Bob has no
// Tag, so he takes the Branch's otherwise.
func TestExternalAutomationReportGivesEachForkPath(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	vip, otherwise := automationstep.NewID(), automationstep.NewID()
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths: []automationstep.BranchPath{{ID: vip, Condition: conditiontree.Branch{
			Kind: conditiontree.KindGroup, Combinator: conditiontree.And,
			Entries: []json.RawMessage{json.RawMessage(`{"kind":"tag","name":"VIP","operator":"has"}`)},
		}, Steps: []automationstep.Step{}}},
		Otherwise: automationstep.Path{ID: otherwise, Steps: []automationstep.Step{}},
	}}
	root := automationstep.Root(branch)
	a, err := m.Create(ctx, acme, automations.CreateInput{Name: "Forking", Trigger: eventmatcher.Matcher{Action: "fork.entered"}, Steps: &root})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, a.ID, testhelper.AutomationRevisionIn(ctx, t, acme, a.ID))
	require.NoError(t, err)
	stepper := testhelper.Module[*automations.StepExecution](env)
	err = stepper.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactBobID, Event: automations.Event{Action: "fork.entered", OccurredAt: time.Now()}})
	require.NoError(t, err)
	env.RunQueuedSteps(t, stepper)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead)

	report := automationReport(t, c, externalapi.AutomationsReportParams{ID: entityIDString(a.ID)})

	assert.Equal(t, []externalapi.AutomationPathFigures{
		{StepId: externalapi.StepId(branch.ID), PathId: externalapi.StepId(otherwise), Enrollments: 1},
	}, report.Paths)
}
