package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An End inside a fork's path completes the Enrollment there (ADR 0124): the Steps after the fork
// never run for it. Alice, with the Tag, takes the path that ends; Bob, without, takes otherwise and
// goes on to the Step after the Branch.
func TestAnEndInAForksPathCompletesTheEnrollmentBeforeTheStepsAfterTheFork(t *testing.T) {
	env := testhelper.Setup(t)
	ends, otherwise := automationstep.NewID(), automationstep.NewID()
	end := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.End{}}
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(ends, condOf(t, hasTag("ends-early")), tagStep("before-end"), end)},
		Otherwise: plainPath(otherwise),
	}}
	after := tagStep("after-fork")
	auto := publishedBranching(t, env, fixtures.AcmeID, branch, after)
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, "ends-early")
	alice := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
	bob := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactBobID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	aliceRun := env.DB.AutomationRun.GetX(t.Context(), alice)
	assert.Equal(t, automationrun.StatusCompleted, aliceRun.Status)
	assert.Equal(t, end.ID, lo.FromPtr(aliceRun.CurrentStepID), "it ended on the End inside the path")
	assert.NotNil(t, recordOf(t, env, alice, end.ID).LeftAt, "the End is left as it completes the Enrollment")
	assert.NotContains(t, reachedSteps(t, env, alice), after.ID, "the Step after the fork does not run")
	assert.NotNil(t, recordOf(t, env, bob, after.ID).LeftAt, "the path without an End goes on after the fork")
}

// reachedSteps is the ids of the Steps the Enrollment reached, in order.
func reachedSteps(t *testing.T, env *testhelper.TestEnv, runID int64) []automationstep.ID {
	t.Helper()
	return lo.Map(stepRecordsOf(t, env, runID), func(r *ent.AutomationStepRecord, _ int) automationstep.ID { return r.StepID })
}

// kept is what a Step record says of the Enrollment's way through the Step.
type kept struct {
	step      automationstep.ID
	path      automationstep.ID
	outcomes  map[automationstep.ID]conditiontree.Truth
	reachedAt time.Time
	leftAt    time.Time
}

// place is where the Enrollment stands and what it kept of every Step it reached.
type place struct {
	status  automationrun.Status
	current automationstep.ID
	records []kept
	queued  []map[string]any
}

// placeOf reads the Enrollment's place, its Step records and the step jobs queued.
func placeOf(t *testing.T, env *testhelper.TestEnv, runID int64) place {
	t.Helper()
	run := env.DB.AutomationRun.GetX(t.Context(), runID)
	records := lo.Map(stepRecordsOf(t, env, runID), func(r *ent.AutomationStepRecord, _ int) kept {
		return kept{r.StepID, lo.FromPtr(r.PathID), r.PathOutcomes, r.ReachedAt, lo.FromPtr(r.LeftAt)}
	})
	return place{run.Status, lo.FromPtr(run.CurrentStepID), records, queuedSteps(t, env)}
}

// holding is a Wait that keeps the Enrollment on its path while a test delivers the fork's job
// again.
func holding() automationstep.Step { return automationstep.New(automationstep.Wait{Seconds: 3600}) }

// A Branch's job delivered again for the same Enrollment does not decide it again (#1564): Alice,
// without the Tag, took otherwise; she has the Tag by the time the job comes again, and still her
// record, her place on otherwise and the queued jobs are as they were.
func TestABranchDeliveredAgainKeepsThePathTaken(t *testing.T) {
	env := testhelper.Setup(t)
	tagged, otherwise := automationstep.NewID(), automationstep.NewID()
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(tagged, condOf(t, hasTag("redelivered")), holding())},
		Otherwise: plainPath(otherwise, holding()),
	}}
	auto := publishedBranching(t, env, fixtures.AcmeID, branch)
	alice := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
	stepper := stepperOf(env, resolvingTo(&fakeSender{}))
	require.True(t, env.RunNextStep(t, stepper), "the Branch's job")
	before := placeOf(t, env, alice)
	require.Equal(t, otherwise, tookPath(recordOf(t, env, alice, branch.ID)))
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, "redelivered")

	require.NoError(t, stepper.Step(t.Context(), env.DB.Scoped(fixtures.AcmeID), alice, branch.ID))

	assert.Equal(t, before, placeOf(t, env, alice))
}

// A Split's job delivered again for the same Enrollment draws nothing again (#1564, a retry never
// changes the path taken): Alice's record of the Split, her place on the variant and the queued jobs
// are as they were.
func TestASplitDeliveredAgainKeepsThePathTaken(t *testing.T) {
	env := testhelper.Setup(t)
	split := automationstep.Step{ID: pinnedSplitID, Payload: automationstep.Split{Paths: []automationstep.SplitPath{
		{ID: variantPathID, Weight: 50, Steps: []automationstep.Step{holding()}},
		{ID: holdoutPathID, Weight: 50, Steps: []automationstep.Step{holding()}},
	}}}
	auto := publishedSplitting(t, env, split)
	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now())
	alice := onlyEnrollment(t, env, auto.ID, fixtures.ContactAliceID)
	stepper := stepperOf(env, resolvingTo(&fakeSender{}))
	require.True(t, env.RunNextStep(t, stepper), "the Split's job")
	before := placeOf(t, env, alice)
	require.Equal(t, variantPathID, tookPath(recordOf(t, env, alice, pinnedSplitID)))

	require.NoError(t, stepper.Step(t.Context(), env.DB.Scoped(fixtures.AcmeID), alice, pinnedSplitID))

	assert.Equal(t, before, placeOf(t, env, alice))
}
