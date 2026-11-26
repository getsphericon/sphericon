package jobs_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eventrecording"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// branchTrigger is the action that enrolls the Automations of these tests.
const branchTrigger = "branch.entered"

// condOf is a Branch path's Condition: the and-group of the leaves, each the JSON object the
// contract names.
func condOf(t *testing.T, leaves ...map[string]any) conditiontree.Branch {
	t.Helper()
	entries := make([]json.RawMessage, len(leaves))
	for i, l := range leaves {
		raw, err := json.Marshal(l)
		require.NoError(t, err)
		entries[i] = raw
	}
	return conditiontree.Branch{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: entries}
}

// hasTag is a leaf that holds for a Contact with the Tag.
func hasTag(name string) map[string]any {
	return map[string]any{"kind": "tag", "name": name, "operator": "has"}
}

// openedOnce is a leaf that holds for a Contact who opened an email, unknown for one whose address
// is not tracked (ADR 0123).
func openedOnce() map[string]any {
	return map[string]any{"kind": "event", "action": systemevent.EmailOpened, "operator": "atLeast", "times": 1}
}

// tagStep applies the Tag of the name, so a path the Enrollment took shows on the Contact.
func tagStep(name string) automationstep.Step {
	return automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.ApplyTag{Tag: name}}
}

// pathOf is a listed path of a Branch with the id, the Condition and the Steps.
func pathOf(id automationstep.ID, cond conditiontree.Branch, steps ...automationstep.Step) automationstep.BranchPath {
	return automationstep.BranchPath{ID: id, Condition: cond, Steps: append([]automationstep.Step{}, steps...)}
}

// plainPath is a path with the id and the Steps.
func plainPath(id automationstep.ID, steps ...automationstep.Step) automationstep.Path {
	return automationstep.Path{ID: id, Steps: append([]automationstep.Step{}, steps...)}
}

// publishedBranching makes an Automation of the Workspace started by branchTrigger with the root
// path's Steps and publishes it.
func publishedBranching(t *testing.T, env *testhelper.TestEnv, ws int64, steps ...automationstep.Step) *ent.Automation {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(ws)
	m := testhelper.Module[*automations.Module](env)
	root := automationstep.Root(steps...)
	a, err := m.Create(ctx, s, automations.CreateInput{Name: "Branching", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: branchTrigger}}, Steps: &root})
	require.NoError(t, err)
	a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a
}

// enrollIn enrolls the Contact of the Workspace in the Automation through branchTrigger and
// returns its Enrollment.
func enrollIn(t *testing.T, env *testhelper.TestEnv, ws, automationID, contactID int64) int64 {
	t.Helper()
	err := testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(ws),
		automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: branchTrigger, OccurredAt: time.Now()}})
	require.NoError(t, err)
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// tagContact applies the Tag to the Contact of the Workspace.
func tagContact(t *testing.T, env *testhelper.TestEnv, ws, contactID int64, name string) {
	t.Helper()
	_, err := testhelper.Module[*contacts.Module](env).ApplyTag(t.Context(), env.DB.Scoped(ws), contactID, name)
	require.NoError(t, err)
}

// recordOf is what the Enrollment kept of the Step of the id; it fails the test when there is none.
func recordOf(t *testing.T, env *testhelper.TestEnv, runID int64, stepID automationstep.ID) *ent.AutomationStepRecord {
	t.Helper()
	rec, ok := lo.Find(stepRecordsOf(t, env, runID), func(r *ent.AutomationStepRecord) bool { return r.StepID == stepID })
	require.True(t, ok, "the Enrollment reached step %s", stepID)
	return rec
}

// tookPath is the path a fork's record says the Enrollment took, "" for none.
func tookPath(rec *ent.AutomationStepRecord) automationstep.ID { return lo.FromPtr(rec.PathID) }

// The first path whose Condition holds is taken, top to bottom; none holding takes otherwise; the
// Step after the Branch runs whichever path was taken. The Branch's record keeps each path's
// outcome, never a value it tested.
func TestABranchTakesTheFirstPathWhoseConditionHolds(t *testing.T) {
	a, b, otherwise := automationstep.NewID(), automationstep.NewID(), automationstep.NewID()
	cases := []struct {
		name     string
		tags     []string
		want     automationstep.ID
		outcomes map[automationstep.ID]conditiontree.Truth
	}{
		{"both hold", []string{"branch-a", "branch-b"}, a, map[automationstep.ID]conditiontree.Truth{a: conditiontree.TruthTrue, b: conditiontree.TruthTrue}},
		{"only the second holds", []string{"branch-b"}, b, map[automationstep.ID]conditiontree.Truth{a: conditiontree.TruthFalse, b: conditiontree.TruthTrue}},
		{"neither holds", nil, otherwise, map[automationstep.ID]conditiontree.Truth{a: conditiontree.TruthFalse, b: conditiontree.TruthFalse}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
				Paths: []automationstep.BranchPath{
					pathOf(a, condOf(t, hasTag("branch-a")), tagStep("took-a")),
					pathOf(b, condOf(t, hasTag("branch-b")), tagStep("took-b")),
				},
				Otherwise: plainPath(otherwise, tagStep("took-otherwise")),
			}}
			after := tagStep("after")
			auto := publishedBranching(t, env, fixtures.AcmeID, branch, after)
			for _, tag := range tc.tags {
				tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, tag)
			}
			runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

			rec := recordOf(t, env, runID, branch.ID)
			run := env.DB.AutomationRun.GetX(t.Context(), runID)
			assert.Equal(t, automationsteprecord.OutcomePath, lo.FromPtr(rec.Outcome))
			assert.Equal(t, tc.want, tookPath(rec))
			assert.Equal(t, tc.outcomes, rec.PathOutcomes)
			assert.NotNil(t, recordOf(t, env, runID, after.ID).LeftAt, "the Step after the Branch ran")
			assert.Equal(t, automationrun.StatusCompleted, run.Status)
		})
	}
}

// A Branch is decided on the Contact's live data when the Enrollment reaches it: a purchase recorded
// during the wait before it sends the Contact down the path of buyers.
func TestABranchReadsAPurchaseRecordedDuringTheWaitBeforeIt(t *testing.T) {
	env := testhelper.Setup(t)
	bought, otherwise := automationstep.NewID(), automationstep.NewID()
	purchased := map[string]any{"kind": "event", "action": "bought_during_wait", "operator": "atLeast", "times": 1}
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(bought, condOf(t, purchased), tagStep("thanked"))},
		Otherwise: plainPath(otherwise, tagStep("reminded")),
	}}
	auto := publishedBranching(t, env, fixtures.AcmeID, automationstep.New(automationstep.Wait{Seconds: 3600}), branch)
	runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)

	err := testhelper.Module[*eventrecording.Module](env).Record(t.Context(), env.DB.Scoped(fixtures.AcmeID), []eventrecording.Input{
		{SubjectID: "alice-buys", Email: new(fixtures.ContactAliceEmail), Action: "bought_during_wait"},
	})
	require.NoError(t, err)
	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	assert.Equal(t, bought, tookPath(recordOf(t, env, runID, branch.ID)))
}

// An empty path goes on to the Step after its Branch, and a Branch nested in a path is a Branch like
// any: Alice, with both Tags, goes down both nested paths; Bob, with neither, down the empty
// otherwise; both reach the Step after the outer Branch.
func TestAnEmptyPathGoesOnAndANestedBranchWorks(t *testing.T) {
	env := testhelper.Setup(t)
	outer, inner := automationstep.NewID(), automationstep.NewID()
	nested := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(inner, condOf(t, hasTag("nest-y")), tagStep("deep"))},
		Otherwise: plainPath(automationstep.NewID()),
	}}
	otherwise := automationstep.NewID()
	branch := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(outer, condOf(t, hasTag("nest-x")), nested)},
		Otherwise: plainPath(otherwise),
	}}
	after := tagStep("after")
	auto := publishedBranching(t, env, fixtures.AcmeID, branch, after)
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, "nest-x")
	tagContact(t, env, fixtures.AcmeID, fixtures.ContactAliceID, "nest-y")
	alice := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
	bob := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactBobID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	assert.Equal(t, outer, tookPath(recordOf(t, env, alice, branch.ID)))
	assert.Equal(t, inner, tookPath(recordOf(t, env, alice, nested.ID)))
	assert.NotNil(t, recordOf(t, env, alice, after.ID).LeftAt)
	assert.Equal(t, otherwise, tookPath(recordOf(t, env, bob, branch.ID)))
	assert.NotNil(t, recordOf(t, env, bob, after.ID).LeftAt, "the empty otherwise goes on to the Step after the Branch")
}

// A Contact whose address is not tracked has an unknown open count (ADR 0123): the first path not
// false is unknown, so the Enrollment takes the unknown path, even when a later path holds, and the
// record keeps unknown for it. Ann refused tracking and has the Tag VIP.
func TestAnUnknownConditionTakesTheUnknownPathBeforeALaterTrueOne(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), env.DB.Scoped(fixtures.MassiveID), engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.MassiveAnnEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	tagContact(t, env, fixtures.MassiveID, fixtures.MassiveAnnID, "VIP")
	opened, vip, otherwise, unknown := automationstep.NewID(), automationstep.NewID(), automationstep.NewID(), automationstep.NewID()
	onlyOpened := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(opened, condOf(t, openedOnce()), tagStep("took-a"))},
		Otherwise: plainPath(otherwise, tagStep("took-b")),
		Unknown:   new(plainPath(unknown, tagStep("took-u"))),
	}}
	openedOrVIP := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths: []automationstep.BranchPath{
			pathOf(opened, condOf(t, openedOnce()), tagStep("took-a")),
			pathOf(vip, condOf(t, hasTag("VIP")), tagStep("took-c")),
		},
		Otherwise: plainPath(otherwise, tagStep("took-b")),
		Unknown:   new(plainPath(unknown, tagStep("took-u"))),
	}}
	cases := []struct {
		name     string
		branch   automationstep.Step
		outcomes map[automationstep.ID]conditiontree.Truth
	}{
		{"one path", onlyOpened, map[automationstep.ID]conditiontree.Truth{opened: conditiontree.TruthUnknown}},
		{"a later path holds", openedOrVIP, map[automationstep.ID]conditiontree.Truth{opened: conditiontree.TruthUnknown, vip: conditiontree.TruthTrue}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			auto := publishedBranching(t, env, fixtures.MassiveID, tc.branch)
			runID := enrollIn(t, env, fixtures.MassiveID, auto.ID, fixtures.MassiveAnnID)

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

			rec := recordOf(t, env, runID, tc.branch.ID)
			assert.Equal(t, unknown, tookPath(rec))
			assert.Equal(t, tc.outcomes, rec.PathOutcomes)
		})
	}
}
