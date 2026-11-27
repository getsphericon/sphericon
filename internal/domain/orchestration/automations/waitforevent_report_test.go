package automations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The report counts each path of a Wait for event as it counts a Branch's and a Split's: by path id
// across versions, the Enrollments that took it and how many of them reached the goal (ADR 0124).

// waitGoal is the action of the goal Event of these tests' Automations, which none of them awaits.
const waitGoal = "goal.reached"

// waitPaths is the Automation's report of its forks' paths.
func (w waiting) waitPaths(t *testing.T) []automations.PathFigures {
	t.Helper()
	report, err := testhelper.Module[*automations.Module](w.env).ReportOf(t.Context(), w.env.DB.Scoped(fixtures.AcmeID), w.automationID, automations.ReportPeriod7Days)
	require.NoError(t, err)
	return report.Paths
}

// arrive records the awaited Event of the Contact, occurred now.
func (w waiting) arrive(t *testing.T, contactID int64) {
	t.Helper()
	handStored(t, w.env, contactID, "order.created", collected(contactID, "order.created", w.clock.Now(), nil))
}

// reachGoal records the goal Event of the Contact, occurred now.
func (w waiting) reachGoal(t *testing.T, contactID int64) {
	t.Helper()
	handStored(t, w.env, contactID, waitGoal, collected(contactID, waitGoal, w.clock.Now(), nil))
}

// Three Enrollments take `arrived` and one of them then reaches the goal; two take `timed_out` at
// the deadline. The report shows 3/1 for `arrived` and 2/0 for `timed_out`, in the Step's order.
func TestTheReportCountsEachWaitForEventPathsEnrollmentsAndGoals(t *testing.T) {
	w := waitFor(t, "trial.started", []string{"order.created"}, &eventmatcher.Matcher{Action: waitGoal})
	buyer, _ := w.enroll(t, "trial.started")
	secondBuyer, _ := w.enroll(t, "trial.started")
	thirdBuyer, _ := w.enroll(t, "trial.started")
	_, firstIdle := w.enroll(t, "trial.started")
	_, secondIdle := w.enroll(t, "trial.started")

	w.clock.Advance(10 * time.Minute)
	w.arrive(t, buyer.ID)
	w.arrive(t, secondBuyer.ID)
	w.arrive(t, thirdBuyer.ID)
	w.reachGoal(t, buyer.ID)
	w.clock.Advance(waitTimeout)
	w.timeout(t, firstIdle)
	w.timeout(t, secondIdle)
	paths := w.waitPaths(t)

	assert.Equal(t, []automations.PathFigures{
		{StepID: w.wait, PathID: w.pathOf(t, w.arrived), Enrollments: 3, GoalReached: 1},
		{StepID: w.wait, PathID: w.pathOf(t, w.timedOut), Enrollments: 2},
	}, paths)
}

// A path's figures stay under its path id when a later version moves the Step. Version 1 holds the
// Wait for event first, and one Contact takes `arrived` there and reaches the goal; version 2 keeps
// the Step and its paths' ids behind a minute's wait, and another Contact takes `arrived` there.
// The report sums both under the one path.
func TestAWaitForEventPathKeepsItsFiguresWhenALaterVersionMovesTheStep(t *testing.T) {
	w := waitFor(t, "trial.started", []string{"order.created"}, &eventmatcher.Matcher{Action: waitGoal})
	ctx := t.Context()
	acme := w.env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](w.env)
	arrivedPath := w.pathOf(t, w.arrived)
	first, firstRun := w.enroll(t, "trial.started")
	w.clock.Advance(time.Minute)
	w.arrive(t, first.ID)
	w.reachGoal(t, first.ID)

	live, err := m.Get(ctx, acme, w.automationID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	moved := automationstep.Root(append([]automationstep.Step{automationstep.New(automationstep.Wait{Seconds: 60})}, version.Steps.Steps...)...)
	_, err = m.Update(ctx, acme, w.automationID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, acme, w.automationID), Steps: &moved})
	require.NoError(t, err)
	_, err = m.Publish(ctx, acme, w.automationID, testhelper.AutomationRevisionIn(ctx, t, acme, w.automationID))
	require.NoError(t, err)
	second, secondRun := w.enroll(t, "trial.started")
	minute := w.standsOn(t, secondRun)
	w.clock.Advance(time.Minute)
	require.NoError(t, testhelper.Module[*automations.StepExecution](w.env).Step(ctx, acme, secondRun, minute))
	reached := w.standsOn(t, secondRun)
	w.clock.Advance(time.Minute)
	w.arrive(t, second.ID)
	paths := w.waitPaths(t)

	assert.NotEqual(t, w.wait, minute, "version 2 holds a Step before the Wait for event")
	assert.Equal(t, w.wait, reached, "version 2 reaches the moved Wait for event after its minute")
	assert.Equal(t, 1, versionOfRun(t, w.env, firstRun))
	assert.Equal(t, 2, versionOfRun(t, w.env, secondRun))
	assert.Equal(t, []automations.PathFigures{
		{StepID: w.wait, PathID: arrivedPath, Enrollments: 2, GoalReached: 1},
	}, paths)
}
