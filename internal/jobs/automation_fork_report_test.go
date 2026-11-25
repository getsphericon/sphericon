package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The report gives each path of a fork the Enrollments that took it and how many of them reached
// the goal (ADR 0124).

// forkGoal is the action of the goal Event of these tests' Automations.
const forkGoal = "fork.goal"

// publishedWithGoal makes an Automation of Acme started by branchTrigger, with forkGoal as its goal
// Event and the root path's Steps, and publishes it.
func publishedWithGoal(t *testing.T, env *testhelper.TestEnv, steps ...automationstep.Step) int64 {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	root := automationstep.Root(steps...)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Forking", Trigger: eventmatcher.Matcher{Action: branchTrigger},
		GoalEvent: &eventmatcher.Matcher{Action: forkGoal}, Steps: &root,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a.ID
}

// pathFigures is the Automation's report of its forks' paths, read through /api.
func pathFigures(t *testing.T, env *testhelper.TestEnv, automationID int64) []externalapi.AutomationPathFigures {
	t.Helper()
	return automationReport(t, env, automationID).Paths
}

// Each path of a Branch, its unknown path included, counts the Enrollments that took it and those
// of them that reached the goal; a path none took is not listed. Alice has the Tag of the first
// path and reaches the goal at the call after the Branch; Bob has no Tag and takes otherwise;
// Carol has the Tag of the second path, whose open test is unknown for her untracked address, so
// she takes the unknown path.
func TestTheReportCountsEachBranchPathsEnrollmentsAndGoals(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(ctx, acme, engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.ContactCarolEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	first, second, otherwise, unknown := automationstep.NewID(), automationstep.NewID(), automationstep.NewID(), automationstep.NewID()
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths: []automationstep.BranchPath{
			pathOf(first, condOf(t, hasTag("fork-first")), tagStep("took-first")),
			pathOf(second, condOf(t, hasTag("fork-second"), openedOnce()), tagStep("took-second")),
		},
		Otherwise: plainPath(otherwise, tagStep("took-otherwise")),
		Unknown:   new(plainPath(unknown)),
	}}
	// The queued Steps run up to the HTTP call after the Branch, whose job they leave queued: each
	// Enrollment stays active there, for the goal.
	call := automationstep.New(automationstep.HTTP{URL: "https://hooks.example.com/forked", Body: `{"ok":true}`})
	automationID := publishedWithGoal(t, env, branch, call)
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, "fork-first")
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactCarolID, "fork-second")
	enrollIn(t, env, fixtures.AcmeID, automationID, fixtures.ContactAliceID)
	enrollIn(t, env, fixtures.AcmeID, automationID, fixtures.ContactBobID)
	enrollIn(t, env, fixtures.AcmeID, automationID, fixtures.ContactCarolID)
	stepper := stepperOf(env, resolvingTo(&fakeSender{}))
	env.RunQueuedSteps(t, stepper)

	err = testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, acme, automations.OnEventInput{
		ContactID: fixtures.ContactAliceID, Event: automations.Event{Action: forkGoal, OccurredAt: time.Now()},
	})
	require.NoError(t, err)
	env.RunQueuedSteps(t, stepper)
	paths := pathFigures(t, env, automationID)

	assert.Equal(t, []externalapi.AutomationPathFigures{
		{StepId: externalapi.StepId(branch.ID), PathId: externalapi.StepId(first), Enrollments: 1, GoalReached: 1},
		{StepId: externalapi.StepId(branch.ID), PathId: externalapi.StepId(otherwise), Enrollments: 1},
		{StepId: externalapi.StepId(branch.ID), PathId: externalapi.StepId(unknown), Enrollments: 1},
	}, paths)
}
