package site_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func automationResults(t *testing.T, c *siteapi.Client, params siteapi.SiteAutomationReportsGetParams) siteapi.AutomationResults {
	t.Helper()
	res, err := c.SiteAutomationReportsGet(t.Context(), params)
	require.NoError(t, err)
	report := res
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

	exited, err := env.ExitEnrollment(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationTagOnEngagementID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, exited)
	after := automationResults(t, c, params)

	assert.Equal(t, siteapi.AutomationReportPeriod7d, after.Period)
	assert.Len(t, after.Series, 7)
	assert.Equal(t, before.Current.Exited+1, after.Current.Exited)
	assert.Equal(t, before.Previous, after.Previous)
}

// The report's period ends on today by the Instance clock, not the wall's: ten days ahead, the
// last day of the series is that clock's day in the Workspace time zone.
func TestTheAutomationReportEndsOnTheInstanceClocksDay(t *testing.T) {
	at := time.Now().AddDate(0, 0, 10)
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return at }))
	loc, err := reportday.Location(env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).Timezone)
	require.NoError(t, err)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)
	today := at.In(loc).Format(time.DateOnly)

	results := automationResults(t, c, siteapi.SiteAutomationReportsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: idStr(fixtures.AutomationWelcomeSeriesID),
	})

	require.NotEmpty(t, results.Series)
	last := results.Series[len(results.Series)-1]
	assert.Equal(t, today, last.Date, "the series ends on the Instance clock's day")
}

func TestTheAutomationReportReadsThirtyDaysByDefault(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	results := automationResults(t, c, siteapi.SiteAutomationReportsGetParams{
		WorkspaceId:  fixtures.AcmePublicID,
		AutomationId: idStr(fixtures.AutomationWelcomeSeriesID),
	})

	assert.Equal(t, siteapi.AutomationReportPeriod30d, results.Period)
	assert.Len(t, results.Series, 30)
}

// The console reads each fork path's Enrollments and goals in the report (ADR 0124): Bob has no
// Tag, so he takes the Branch's otherwise.
func TestTheAutomationReportGivesTheConsoleEachForkPath(t *testing.T) {
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
	a, err := m.Create(ctx, acme, automations.CreateInput{Name: "Forking", Trigger: automations.Trigger{Event: eventmatcher.Matcher{Action: "fork.entered"}}, Steps: &root})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, a.ID, testhelper.AutomationRevisionIn(ctx, t, acme, a.ID))
	require.NoError(t, err)
	stepper := testhelper.Module[*automations.StepExecution](env)
	err = stepper.OnEvent(ctx, acme, automations.OnEventInput{ContactID: fixtures.ContactBobID, Event: automations.Event{Action: "fork.entered", OccurredAt: time.Now()}})
	require.NoError(t, err)
	env.RunQueuedSteps(t, stepper)

	report, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteAutomationReportsGet(ctx, siteapi.SiteAutomationReportsGetParams{
		WorkspaceId: fixtures.AcmePublicID, AutomationId: idStr(a.ID),
	})
	require.NoError(t, err)

	assert.Equal(t, []siteapi.AutomationPathFigures{
		{StepId: siteapi.StepId(branch.ID), PathId: siteapi.StepId(otherwise), Enrollments: 1},
	}, report.Paths)
}
