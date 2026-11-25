package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// splitTrigger is the action that enrolls the Automations of these tests.
const splitTrigger = "split.entered"

// The Split of these tests and its paths. Its id is pinned: a Contact's draw at a Split is a hash
// of the Contact's id, the Split's id and the Enrollment's ordinal (computed apart from the code
// with shasum), and at this id Alice's first Enrollment draws 39, Bob's first 96 and Alice's
// second 91. Of a 50/50 Split, 0 to 49 take the first path, the variant, and 50 to 99 the second,
// the holdout.
const (
	pinnedSplitID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000006")
	variantPathID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000101")
	holdoutPathID = automationstep.ID("5c1d0e2f-8a4b-4c3d-9e2f-000000000102")
)

// fiftyFifty is a Split of the pinned id whose first path, of weight 50, sends an email and whose
// second, of weight 50, is an empty holdout.
func fiftyFifty() automationstep.Step {
	return automationstep.Step{ID: pinnedSplitID, Payload: automationstep.Split{Paths: []automationstep.SplitPath{
		{ID: variantPathID, Weight: 50, Steps: []automationstep.Step{
			automationstep.New(automationstep.Email{Subject: "Try this", Body: "<p>A variant.</p>"}),
		}},
		{ID: holdoutPathID, Weight: 50, Steps: []automationstep.Step{}},
	}}}
}

// publishedSplitting makes an Automation of Acme started by splitTrigger that enrolls a Contact
// again once its Enrollment ended, with the root path's Steps, and publishes it.
func publishedSplitting(t *testing.T, env *testhelper.TestEnv, steps ...automationstep.Step) *ent.Automation {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	root := automationstep.Root(steps...)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Splitting", Trigger: eventmatcher.Matcher{Action: splitTrigger},
		Entry: automations.Entry{Policy: automationversion.EntryPolicyAgain}, Steps: &root,
	})
	require.NoError(t, err)
	a, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a
}

// enterSplitAt hands the Contact's splitTrigger, occurred at, to the Automations of Acme.
func enterSplitAt(t *testing.T, env *testhelper.TestEnv, contactID int64, at time.Time) {
	t.Helper()
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: splitTrigger, OccurredAt: at}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// onlyEnrollment is the one Enrollment of the Contact in the Automation.
func onlyEnrollment(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// sentBy is the subjects of the email the Enrollment sent.
func sentBy(t *testing.T, env *testhelper.TestEnv, runID int64) []string {
	t.Helper()
	msgs := env.DB.OutboundMessage.Query().Where(outboundmessage.AutomationRunID(runID)).AllX(t.Context())
	return lo.Map(msgs, func(m *ent.OutboundMessage, _ int) string { return lo.FromPtr(m.Subject) })
}

// A Split 50/50 with an empty path sends nothing to the holdout: Alice draws the variant and is sent
// its email, Bob draws the holdout and is sent nothing; both go on to the Step after the Split, and
// the Split's record keeps the path each took.
func TestASplitSendsNothingToItsHoldout(t *testing.T) {
	env := testhelper.Setup(t)
	after := tagStep("after-split")
	auto := publishedSplitting(t, env, fiftyFifty(), after)
	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now())
	enterSplitAt(t, env, fixtures.ContactBobID, time.Now())
	alice := onlyEnrollment(t, env, auto.ID, fixtures.ContactAliceID)
	bob := onlyEnrollment(t, env, auto.ID, fixtures.ContactBobID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

	aliceSplit := recordOf(t, env, alice, pinnedSplitID)
	bobSplit := recordOf(t, env, bob, pinnedSplitID)
	assert.Equal(t, automationsteprecord.OutcomePath, lo.FromPtr(bobSplit.Outcome))
	assert.Equal(t, variantPathID, tookPath(aliceSplit))
	assert.Equal(t, holdoutPathID, tookPath(bobSplit))
	assert.Nil(t, bobSplit.PathOutcomes, "a Split tests no Condition")
	assert.Equal(t, []string{"Try this"}, sentBy(t, env, alice))
	assert.Empty(t, sentBy(t, env, bob), "the holdout is sent nothing")
	assert.NotNil(t, recordOf(t, env, alice, after.ID).LeftAt)
	assert.NotNil(t, recordOf(t, env, bob, after.ID).LeftAt, "the holdout goes on to the Step after the Split")
	bobRun := env.DB.AutomationRun.GetX(t.Context(), bob)
	assert.Equal(t, automationrun.StatusCompleted, bobRun.Status)
}

// The path is a draw of the Contact, the Split and the Enrollment's ordinal, with no random state:
// the same Enrollment lands on the same path wherever the Split runs for it (here, a second
// Automation holding the same Split), and a new Enrollment of the Contact draws again. Alice's first
// Enrollment takes the variant in both Automations; her second takes the holdout.
func TestASplitDrawsOncePerEnrollmentAndAgainForANewOne(t *testing.T) {
	env := testhelper.Setup(t)
	auto := publishedSplitting(t, env, fiftyFifty())
	twin := publishedSplitting(t, env, fiftyFifty())
	// Its Steps hold no wait: they run on the wall clock, so the first Enrollment ends before the
	// second begins.
	stepper := stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), time.Now), time.Now)
	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now().Add(-2*time.Hour))
	env.RunQueuedSteps(t, stepper)

	enterSplitAt(t, env, fixtures.ContactAliceID, time.Now())
	env.RunQueuedSteps(t, stepper)

	runs := byOrdinalOf(t, env, auto.ID, fixtures.ContactAliceID)
	twinRuns := byOrdinalOf(t, env, twin.ID, fixtures.ContactAliceID)
	require.Len(t, runs, 2)
	require.Len(t, twinRuns, 2)
	first := tookPath(recordOf(t, env, runs[0].ID, pinnedSplitID))
	twinFirst := tookPath(recordOf(t, env, twinRuns[0].ID, pinnedSplitID))
	second := tookPath(recordOf(t, env, runs[1].ID, pinnedSplitID))
	assert.Equal(t, variantPathID, first)
	assert.Equal(t, first, twinFirst, "the same Enrollment's draw lands on the same path")
	assert.Equal(t, holdoutPathID, second, "a new Enrollment draws again")
	assert.Equal(t, []string{"Try this"}, sentBy(t, env, runs[0].ID))
	assert.Empty(t, sentBy(t, env, runs[1].ID))
}
