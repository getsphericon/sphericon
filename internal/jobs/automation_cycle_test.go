package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationskippedentry"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The cycle guard (ADR 0127), through the jobs that carry a chain from an Event to the Steps it
// causes: Ouroboros's Automations cause each other's Triggers, and an entry whose cause already
// passed through the Automation is skipped as `cycle`.

// ouroborosAutomations are the Automations of the Ouroboros fixture Workspace, all active.
var ouroborosAutomations = []int64{
	fixtures.AutomationOuroborosUntagRingID,
	fixtures.AutomationOuroborosTagRingID,
	fixtures.AutomationOuroborosWaitThenUntagRingID,
	fixtures.AutomationOuroborosSelfTaggerID,
	fixtures.AutomationOuroborosStarterID,
	fixtures.AutomationOuroborosOnCompletedID,
	fixtures.AutomationOuroborosOnClickID,
	fixtures.AutomationOuroborosGoalID,
}

// onlyActive deactivates every Automation of Ouroboros but keep, so that only the chain a test is
// about runs.
func onlyActive(t *testing.T, env *testhelper.TestEnv, keep ...int64) {
	t.Helper()
	for _, id := range lo.Without(ouroborosAutomations, keep...) {
		_, err := testhelper.Module[*automations.Module](env).Deactivate(t.Context(), ouroboros(env), id)
		require.NoError(t, err)
	}
}

// ouroboros is the scope of the Ouroboros fixture Workspace.
func ouroboros(env *testhelper.TestEnv) *ent.Scoped {
	return env.DB.Scoped(fixtures.OuroborosID)
}

// tagIn applies the Tag to the Ouroboros Contact in ctx, as a request would.
func tagIn(ctx context.Context, t *testing.T, env *testhelper.TestEnv, contactID int64, name string) {
	t.Helper()
	_, err := testhelper.Module[*contacts.Module](env).ApplyTag(ctx, ouroboros(env), contactID, name)
	require.NoError(t, err)
}

// skipsOf is why each of the Contact's entries into the Automation was skipped, newest first.
func skipsOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []automationskippedentry.Reason {
	t.Helper()
	page, err := testhelper.Module[*automations.Module](env).SkippedEntries(t.Context(), ouroboros(env),
		pagination.Params{Page: 1, PageSize: 100}, automations.SkippedEntriesFilter{AutomationID: automationID, ContactID: contactID})
	require.NoError(t, err)
	return lo.Map(page.Items, func(e *ent.AutomationSkippedEntry, _ int) automationskippedentry.Reason { return e.Reason })
}

// A removes the Tag whose application starts it and B applies it back: B's Tag, caused by A's
// Step, does not enroll Uma in A again, and the skip says `cycle`. Otto's Tag, applied later in the
// same request (one correlation id) but caused by no Automation, still enrolls him in A.
func TestAChainThatComesBackToAnAutomationIsSkippedAsACycle(t *testing.T) {
	env := testhelper.Setup(t)
	a, b := int64(fixtures.AutomationOuroborosUntagRingID), int64(fixtures.AutomationOuroborosTagRingID)
	onlyActive(t, env, a, b)
	request := logging.WithRequestID(t.Context(), "req-ring")

	tagIn(request, t, env, fixtures.ContactOuroborosUmaID, "ring")
	env.PerformEnqueuedJobs(t)
	tagIn(request, t, env, fixtures.ContactOuroborosOttoID, "ring")
	env.PerformEnqueuedJobs(t)

	umaInA := enrollmentsOf(t, env, a, fixtures.ContactOuroborosUmaID)
	umaInB := enrollmentsOf(t, env, b, fixtures.ContactOuroborosUmaID)
	umaSkips := skipsOf(t, env, a, fixtures.ContactOuroborosUmaID)
	ottoInA := enrollmentsOf(t, env, a, fixtures.ContactOuroborosOttoID)
	assert.Len(t, umaInA, 1)
	assert.Len(t, umaInB, 1, "A's Step enrolled Uma in B")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCycle}, umaSkips)
	assert.Len(t, ottoInA, 1, "an Event no Automation caused enrolls, whatever request it shares")
}

// An Automation on `contact.tagged` whose Step applies a Tag: its own `contact.tagged` never enrolls
// Uma in it again. Its Enrollment is still active then, waiting, and the skip is `cycle`, the
// earlier check, rather than `active_enrollment`.
func TestAnAutomationsOwnTagIsSkippedAsACycleRatherThanAnActiveEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	selfTagger := int64(fixtures.AutomationOuroborosSelfTaggerID)
	onlyActive(t, env, selfTagger)

	tagIn(t.Context(), t, env, fixtures.ContactOuroborosUmaID, "start")
	env.PerformEnqueuedJobs(t)

	enrolled := enrollmentsOf(t, env, selfTagger, fixtures.ContactOuroborosUmaID)
	skips := skipsOf(t, env, selfTagger, fixtures.ContactOuroborosUmaID)
	require.Len(t, enrolled, 1)
	run := env.DB.AutomationRun.GetX(t.Context(), enrolled[0])
	assert.Equal(t, automationrun.StatusActive, run.Status, "the Enrollment waits after its Tag")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCycle}, skips)
}

// An Automation on `automation.completed`: Starter completes at once and enrolls Uma in it, which
// completes at once too, and its own `automation.completed` never enrolls her again. Its Enrollment
// ended, so `once` holds as well; the skip is `cycle`.
func TestAnAutomationsOwnCompletionIsSkippedAsACycle(t *testing.T) {
	env := testhelper.Setup(t)
	starter, onCompleted := int64(fixtures.AutomationOuroborosStarterID), int64(fixtures.AutomationOuroborosOnCompletedID)
	onlyActive(t, env, starter, onCompleted)

	publishIn(t.Context(), t, env, &events.CollectedEvent{WorkspaceID: fixtures.OuroborosID, ContactID: fixtures.ContactOuroborosUmaID, Action: "ouro_start"})
	env.PerformEnqueuedJobs(t)

	enrolled := enrollmentsOf(t, env, onCompleted, fixtures.ContactOuroborosUmaID)
	skips := skipsOf(t, env, onCompleted, fixtures.ContactOuroborosUmaID)
	assert.Len(t, enrolled, 1, "Starter's completion enrolls her")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCycle}, skips)
}

// A waits an hour before it removes the Tag, and B applies it back: the chain survives the wait,
// which a Step resumed later reads from its Enrollment, and the entry into A is still `cycle`.
func TestAChainThroughAWaitIsStillSkippedAsACycle(t *testing.T) {
	env := testhelper.Setup(t)
	a, b := int64(fixtures.AutomationOuroborosWaitThenUntagRingID), int64(fixtures.AutomationOuroborosTagRingID)
	onlyActive(t, env, a, b)
	tagIn(t.Context(), t, env, fixtures.ContactOuroborosUmaID, "ring")
	env.PerformEnqueuedJobs(t)
	waiting := enrollmentsOf(t, env, a, fixtures.ContactOuroborosUmaID)
	require.Len(t, waiting, 1)

	ran := env.RunNextStep(t, stepperOf(env, resolvingTo(&fakeSender{})))
	env.PerformEnqueuedJobs(t)

	require.True(t, ran, "the wait was queued")
	umaInB := enrollmentsOf(t, env, b, fixtures.ContactOuroborosUmaID)
	skips := skipsOf(t, env, a, fixtures.ContactOuroborosUmaID)
	assert.Len(t, umaInB, 1, "A's Step after the wait enrolled Uma in B")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCycle}, skips)
}

// Uma reaches Goal's goal, and a Rule on `automation.goal_reached` enrolls her in Goal: the Rule's
// Enroll is skipped as `cycle`, since the goal it fired on was Goal's own.
func TestARulesEnrollBackIntoTheAutomationItsEventCameFromIsSkippedAsACycle(t *testing.T) {
	env := testhelper.Setup(t)
	goal := int64(fixtures.AutomationOuroborosGoalID)
	onlyActive(t, env, goal)
	publishIn(t.Context(), t, env, &events.CollectedEvent{WorkspaceID: fixtures.OuroborosID, ContactID: fixtures.ContactOuroborosUmaID, Action: "ouro_go"})
	env.PerformEnqueuedJobs(t)

	publishIn(t.Context(), t, env, &events.CollectedEvent{WorkspaceID: fixtures.OuroborosID, ContactID: fixtures.ContactOuroborosUmaID, Action: "ouro_paid"})
	env.PerformEnqueuedJobs(t)

	enrolled := enrollmentsOf(t, env, goal, fixtures.ContactOuroborosUmaID)
	require.Len(t, enrolled, 1)
	run := env.DB.AutomationRun.GetX(t.Context(), enrolled[0])
	skips := skipsOf(t, env, goal, fixtures.ContactOuroborosUmaID)
	firings := env.DB.RuleFiring.Query().Where(rulefiring.RuleID(fixtures.RuleOuroborosEnrollOnGoalID)).CountX(t.Context())
	assert.Equal(t, automationrun.StatusGoalReached, run.Status)
	assert.Equal(t, 1, firings, "the Rule fired on the goal")
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonCycle}, skips)
}

// A click of the Automation's own email carries no chain, so the guard does not see it: it runs the
// entry checks like any Event. While Uma's Enrollment is active it is skipped as
// `active_enrollment`, and after the Enrollment ended, under `once`, as `once`.
func TestAClickOnTheAutomationsOwnEmailIsBoundedByTheEntryChecks(t *testing.T) {
	env := testhelper.Setup(t)
	onClick := int64(fixtures.AutomationOuroborosOnClickID)
	onlyActive(t, env, onClick)
	click := func(at time.Time) *events.EmailEngagement {
		return &events.EmailEngagement{
			Action: systemevent.EmailClicked, WorkspaceID: fixtures.OuroborosID, ContactID: fixtures.ContactOuroborosUmaID,
			Email: fixtures.ContactOuroborosUmaEmail, AutomationID: onClick, ObservedAt: at,
		}
	}
	publishIn(t.Context(), t, env, click(time.Now().Add(-2*time.Hour)))
	env.PerformEnqueuedJobs(t)

	publishIn(t.Context(), t, env, click(time.Now().Add(-time.Hour)))
	env.PerformEnqueuedJobs(t)
	whileActive := skipsOf(t, env, onClick, fixtures.ContactOuroborosUmaID)
	exited, err := env.ExitEnrollment(t.Context(), ouroboros(env), onClick, fixtures.ContactOuroborosUmaID)
	require.NoError(t, err)
	publishIn(t.Context(), t, env, click(time.Now().Add(time.Minute)))
	env.PerformEnqueuedJobs(t)

	enrolled := enrollmentsOf(t, env, onClick, fixtures.ContactOuroborosUmaID)
	afterEnd := skipsOf(t, env, onClick, fixtures.ContactOuroborosUmaID)
	assert.Len(t, enrolled, 1, "the first click enrolled her")
	assert.True(t, exited)
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonActiveEnrollment}, whileActive)
	assert.Equal(t, []automationskippedentry.Reason{automationskippedentry.ReasonOnce, automationskippedentry.ReasonActiveEnrollment}, afterEnd)
}
