package jobs_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// messageTimeout is how long the Wait for event of these tests waits for an open or a click.
const messageTimeout = 48 * time.Hour

// messageWait is a Wait for event on the open or click of a send Step's message: its Step and the
// ids of its arrived, timed out and unknown paths, which tag took-a, took-b and took-u.
type messageWait struct {
	step                       automationstep.Step
	arrived, timedOut, unknown automationstep.ID
}

func waitForMessage(sent automationstep.ID, e conditiontree.Engagement) messageWait {
	w := messageWait{arrived: automationstep.NewID(), timedOut: automationstep.NewID(), unknown: automationstep.NewID()}
	w.step = automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.WaitForEvent{
		Name: "welcome_" + string(e), TimeoutSeconds: int(messageTimeout / time.Second),
		Awaits:   automationstep.AwaitedMessage{Step: sent, Engagement: e},
		Arrived:  plainPath(w.arrived, tagStep("took-a")),
		TimedOut: plainPath(w.timedOut, tagStep("took-b")),
		Unknown:  new(plainPath(w.unknown, tagStep("took-u"))),
	}}
	return w
}

// waitingOnMessage is Alice enrolled in an Automation that sends the welcome email, then waits for
// its open or click, run on a clock the test moves.
type waitingOnMessage struct {
	env     *testhelper.TestEnv
	clock   *harnessClock
	stepper *automations.StepExecution
	email   automationstep.Step
	wait    messageWait
	runID   int64
}

// sentAndWaiting publishes the email and the wait for its message's engagement, enrolls Alice and
// sends her the email, which leaves her on the wait.
func sentAndWaiting(t *testing.T, e conditiontree.Engagement) waitingOnMessage {
	t.Helper()
	env := testhelper.Setup(t)
	email := welcomeEmail()
	wait := waitForMessage(email.ID, e)
	auto := publishedBranching(t, env, fixtures.AcmeID, email, wait.step)
	runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
	clock := &harnessClock{now: time.Now()}
	stepper := stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), time.Now), clock.read)
	ran := env.RunNextStep(t, stepper)
	require.True(t, ran, "the email Step ran")
	return waitingOnMessage{env: env, clock: clock, stepper: stepper, email: email, wait: wait, runID: runID}
}

// messageID is the email the Enrollment was sent.
func (w waitingOnMessage) messageID(t *testing.T) int64 {
	t.Helper()
	return sentMessageOf(t, w.env, w.runID, w.email.ID)
}

// runWait runs the job of the wait queued next, at the clock's moment.
func (w waitingOnMessage) runWait(t *testing.T) {
	t.Helper()
	ran := w.env.RunNextStep(t, w.stepper)
	require.True(t, ran, "the wait's job ran")
}

// atDeadline moves the clock to the wait's deadline, its timeout after the email was sent.
func (w waitingOnMessage) atDeadline() { w.clock.now = w.clock.now.Add(messageTimeout) }

// standsOn is the Step the Enrollment stands on.
func (w waitingOnMessage) standsOn(t *testing.T) automationstep.ID {
	t.Helper()
	return w.env.CurrentStepID(t, w.runID)
}

// took is the path the Enrollment's record of the wait says it took, "" while it waits.
func (w waitingOnMessage) took(t *testing.T) automationstep.ID {
	t.Helper()
	return tookPath(recordOf(t, w.env, w.runID, w.wait.step.ID))
}

// recordOpen records a hit with the user agent on the open pixel of the message, and nothing else:
// the stamp of the open and the Event's trigger job are left queued.
func recordOpen(t *testing.T, env *testhelper.TestEnv, messageID int64, userAgent string) {
	t.Helper()
	token, err := env.Tracker.Token(env.Tracked(t, messageID))
	require.NoError(t, err)
	hit := engagement.Hit{UserAgent: userAgent, Address: "192.0.2.10", MayRecord: func() bool { return true }}
	require.NoError(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), token, hit))
}

// clickLink follows, as a person, a tracked link of the email message.
func clickLink(t *testing.T, env *testhelper.TestEnv, messageID int64) {
	t.Helper()
	link, err := env.Tracker.ClickURL(env.Tracked(t, messageID), "https://dest.test/welcome")
	require.NoError(t, err)
	_, token, _ := strings.Cut(link, "/e/c/")
	resp := follow(t, env, "/e/c/"+token, browserUA)
	require.Equal(t, http.StatusFound, resp.Code)
}

// An open recorded before the Enrollment reaches the wait takes arrived at once, read from the
// recorded open even before the open is stamped on the message.
func TestAWaitForAnOpenTakesArrivedAtOnceWhenTheMessageWasOpenedBeforeIt(t *testing.T) {
	w := sentAndWaiting(t, conditiontree.MessageOpened)
	recordOpen(t, w.env, w.messageID(t), browserUA)

	w.runWait(t)

	assert.Equal(t, w.wait.arrived, w.took(t))
}

// An open or a click recorded during the wait wakes the Enrollment down arrived when its Event is
// handled, before the deadline.
func TestAnEngagementDuringTheWaitTakesArrivedWhenRecorded(t *testing.T) {
	for name, c := range map[string]struct {
		awaits conditiontree.Engagement
		engage func(t *testing.T, env *testhelper.TestEnv, messageID int64)
	}{
		"an open": {conditiontree.MessageOpened, func(t *testing.T, env *testhelper.TestEnv, id int64) {
			t.Helper()
			recordOpen(t, env, id, browserUA)
		}},
		"a click": {conditiontree.MessageClicked, clickLink},
	} {
		t.Run(name, func(t *testing.T) {
			w := sentAndWaiting(t, c.awaits)
			w.runWait(t)
			waiting := w.standsOn(t)

			c.engage(t, w.env, w.messageID(t))
			w.env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})

			assert.Equal(t, w.wait.step.ID, waiting, "it waited until the engagement")
			assert.Equal(t, w.wait.arrived, w.took(t))
		})
	}
}

// A machine's open leaves the Enrollment waiting, and with no open by a person it takes timed out at
// the deadline, not before.
func TestAMachineOpenLeavesTheWaitForAnOpenWaiting(t *testing.T) {
	w := sentAndWaiting(t, conditiontree.MessageOpened)
	w.runWait(t)

	recordOpen(t, w.env, w.messageID(t), scanner)
	w.env.PerformEnqueuedJobs(t, jobs.EvaluateTriggerArgs{})
	w.runWait(t)
	beforeTheDeadline := w.standsOn(t)
	w.atDeadline()
	w.runWait(t)

	assert.Equal(t, w.wait.step.ID, beforeTheDeadline, "still waiting")
	assert.Equal(t, w.wait.timedOut, w.took(t))
}

// The wait takes unknown at once when the app cannot know whether the message was opened: it went
// out untracked (the address refused tracking, or the Workspace tracks no address that granted
// nothing), the send Step was skipped (the address is suppressed), or the Enrollment never passed
// the send Step (it sits on a path of an earlier Branch the Enrollment did not take).
func TestAWaitForAnOpenItCannotKnowOfTakesUnknownAtOnce(t *testing.T) {
	cases := []struct {
		name   string
		before func(t *testing.T, env *testhelper.TestEnv)
		steps  func(email automationstep.Step, w messageWait) []automationstep.Step
	}{
		{"sent untracked", refuseTracking, func(email automationstep.Step, w messageWait) []automationstep.Step {
			return []automationstep.Step{email, w.step}
		}},
		{"sent untracked by the Workspace's dont_track default", dontTrack, func(email automationstep.Step, w messageWait) []automationstep.Step {
			return []automationstep.Step{email, w.step}
		}},
		{"skipped", func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.SuppressDestination(t.Context(), fixtures.ContactAliceEmail)
		}, func(email automationstep.Step, w messageWait) []automationstep.Step {
			return []automationstep.Step{email, w.step}
		}},
		{"never passed", func(*testing.T, *testhelper.TestEnv) {}, func(email automationstep.Step, w messageWait) []automationstep.Step {
			earlier := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
				Paths:     []automationstep.BranchPath{pathOf(automationstep.NewID(), condOf(t, hasTag("never-given")), email)},
				Otherwise: plainPath(automationstep.NewID()),
			}}
			return []automationstep.Step{earlier, w.step}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			email := welcomeEmail()
			wait := waitForMessage(email.ID, conditiontree.MessageOpened)
			auto := publishedBranching(t, env, fixtures.AcmeID, tc.steps(email, wait)...)
			tc.before(t, env)
			runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
			clock := &harnessClock{now: time.Now()}

			env.RunQueuedSteps(t, stepperAt(env, newMod(env, resolvingTo(&fakeSender{}), time.Now), clock.read))

			rec := recordOf(t, env, runID, wait.step.ID)
			assert.Equal(t, wait.unknown, tookPath(rec))
			assert.WithinDuration(t, clock.now, lo.FromPtr(rec.LeftAt), time.Second, "it left at once")
		})
	}
}

// A refusal of tracking recorded during the wait takes unknown at the deadline, not timed out, and
// not before it.
func TestARefusalDuringTheWaitTakesUnknownAtTheDeadline(t *testing.T) {
	w := sentAndWaiting(t, conditiontree.MessageOpened)
	w.runWait(t)

	refuseTracking(t, w.env)
	w.runWait(t)
	beforeTheDeadline := w.standsOn(t)
	w.atDeadline()
	w.runWait(t)

	assert.Equal(t, w.wait.step.ID, beforeTheDeadline, "still waiting")
	assert.Equal(t, w.wait.unknown, w.took(t))
}

// The report counts the Enrollments of each path of the wait, unknown included: Alice is sent the
// email tracked and never opens it, so she times out; Carol refused tracking, so her wait is
// unknown.
func TestTheReportCountsTheUnknownPathOfAWaitForAnOpen(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), env.DB.Scoped(fixtures.AcmeID), engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.ContactCarolEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	email := welcomeEmail()
	wait := waitForMessage(email.ID, conditiontree.MessageOpened)
	auto := publishedBranching(t, env, fixtures.AcmeID, email, wait.step)
	enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
	enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactCarolID)

	env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))
	paths := pathFigures(t, env, auto.ID)

	assert.Equal(t, []externalapi.AutomationPathFigures{
		{StepId: externalapi.StepId(wait.step.ID), PathId: externalapi.StepId(wait.timedOut), Enrollments: 1},
		{StepId: externalapi.StepId(wait.step.ID), PathId: externalapi.StepId(wait.unknown), Enrollments: 1},
	}, paths)
}
