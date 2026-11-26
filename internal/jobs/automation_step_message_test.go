package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages/engagement"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// scanner is the user agent of a scanner of links, whose hit on a message is a machine's; a
// person's is browserUA.
const scanner = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

// messageOf is the test of the message the send Step sent to the Enrollment: opened or clicked.
func messageOf(step automationstep.ID, asked string) map[string]any {
	return map[string]any{"kind": "stepMessage", "step": step, "engagement": asked}
}

// welcomeEmail is an email Step that sends a welcome.
func welcomeEmail() automationstep.Step {
	return automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Email{Subject: "Welcome", Body: "<p>Hello</p>"}}
}

// openedBranch is a Branch on whether the message of the send Step was opened: took-a when it was,
// took-b when it was not, took-u when that is unknown. Its paths are the ids given.
type openedBranch struct {
	step                       automationstep.Step
	opened, otherwise, unknown automationstep.ID
}

func branchOnOpen(t *testing.T, sent automationstep.ID) openedBranch {
	t.Helper()
	b := openedBranch{opened: automationstep.NewID(), otherwise: automationstep.NewID(), unknown: automationstep.NewID()}
	b.step = automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
		Paths:     []automationstep.BranchPath{pathOf(b.opened, condOf(t, messageOf(sent, "opened")), tagStep("took-a"))},
		Otherwise: plainPath(b.otherwise, tagStep("took-b")),
		Unknown:   new(plainPath(b.unknown, tagStep("took-u"))),
	}}
	return b
}

// openAs records a hit with the browser's user agent on the open pixel of the message, and stamps
// what it records on the message, as the message's consumer does.
func openAs(t *testing.T, env *testhelper.TestEnv, messageID int64, userAgent string) {
	t.Helper()
	token, err := env.Tracker.Token(env.Tracked(t, messageID))
	require.NoError(t, err)
	hit := engagement.Hit{UserAgent: userAgent, Address: "192.0.2.10", MayRecord: func() bool { return true }}
	require.NoError(t, testhelper.Module[*engagement.Recorder](env).Open(t.Context(), token, hit))
	env.PerformEnqueuedJobs(t, jobs.MessageObservationArgs{})
}

// refuseTracking records that Alice's address refused tracking, and runs the reach-back that
// withdraws the tracking of her messages already sent (ADR 0120).
func refuseTracking(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	_, err := testhelper.Module[*engagementtracking.Module](env).Record(t.Context(), env.DB.Scoped(fixtures.AcmeID), engagementtracking.RecordInput{
		Channel: channels.Email, Destination: fixtures.ContactAliceEmail, State: engagementtracking.Denied, Provenance: engagementtracking.Manual,
	})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.TrackingWithdrawalArgs{})
}

// dontTrack turns Acme's Engagement tracking default to dont_track, so Alice's address, which
// granted nothing, is not tracked (ADR 0120).
func dontTrack(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	s := env.DB.Scoped(fixtures.AcmeID)
	w, err := s.Workspace(t.Context())
	require.NoError(t, err)
	_, err = testhelper.Module[*workspaces.Module](env).SetEngagementTrackingDefault(t.Context(), s, w.Revision, workspace.EngagementTrackingDefaultDontTrack)
	require.NoError(t, err)
}

// sentMessageOf is the message the Enrollment's run of the send Step left.
func sentMessageOf(t *testing.T, env *testhelper.TestEnv, runID int64, step automationstep.ID) int64 {
	t.Helper()
	id, err := env.DB.OutboundMessage.Query().
		Where(outboundmessage.AutomationRunID(runID), outboundmessage.AutomationStepID(step)).
		OnlyID(t.Context())
	require.NoError(t, err)
	return id
}

// A Branch on whether the Enrollment's own email was opened reads the message that Step sent it,
// once its open is stamped: opened takes the path, sent tracked and not opened by a person (a
// scanner's hit is not an open) takes otherwise, and a message whose tracking was withdrawn after
// the send takes unknown whatever was stamped, as one sent untracked does (ADR 0122).
func TestABranchOnTheEnrollmentsOwnEmailReadsItsOpen(t *testing.T) {
	cases := []struct {
		name  string
		after func(t *testing.T, env *testhelper.TestEnv, messageID int64)
		took  func(b openedBranch) automationstep.ID
		truth conditiontree.Truth
	}{
		{"opened by a person", func(t *testing.T, env *testhelper.TestEnv, id int64) {
			t.Helper()
			openAs(t, env, id, browserUA)
		},
			func(b openedBranch) automationstep.ID { return b.opened }, conditiontree.TruthTrue},
		{"not opened", func(*testing.T, *testhelper.TestEnv, int64) {},
			func(b openedBranch) automationstep.ID { return b.otherwise }, conditiontree.TruthFalse},
		{"opened by a scanner", func(t *testing.T, env *testhelper.TestEnv, id int64) {
			t.Helper()
			openAs(t, env, id, scanner)
		},
			func(b openedBranch) automationstep.ID { return b.otherwise }, conditiontree.TruthFalse},
		{"opened, then its tracking withdrawn", func(t *testing.T, env *testhelper.TestEnv, id int64) {
			t.Helper()
			openAs(t, env, id, browserUA)
			refuseTracking(t, env)
		}, func(b openedBranch) automationstep.ID { return b.unknown }, conditiontree.TruthUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			email := welcomeEmail()
			b := branchOnOpen(t, email.ID)
			auto := publishedBranching(t, env, fixtures.AcmeID, email, b.step)
			runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)
			stepper := stepperOf(env, resolvingTo(&fakeSender{}))
			ran := env.RunNextStep(t, stepper)
			require.True(t, ran, "the email Step ran")
			messageID := sentMessageOf(t, env, runID, email.ID)
			tc.after(t, env, messageID)
			want := tc.took(b)

			env.RunQueuedSteps(t, stepper)

			rec := recordOf(t, env, runID, b.step.ID)
			took := tookPath(rec)
			assert.Equal(t, want, took)
			assert.Equal(t, map[automationstep.ID]conditiontree.Truth{b.opened: tc.truth}, rec.PathOutcomes)
		})
	}
}

// The test is unknown, never false, when the message went out untracked (the address refused
// tracking before the send, or the Workspace tracks no address that granted nothing), when the send Step was skipped (the address is suppressed), and when
// the Enrollment never passed the send Step (it sits on a path of an earlier Branch the Enrollment
// did not take).
func TestABranchOnAMessageTheEnrollmentWasNotSentTrackedTakesUnknown(t *testing.T) {
	cases := []struct {
		name   string
		before func(t *testing.T, env *testhelper.TestEnv)
		steps  func(email automationstep.Step, b openedBranch) []automationstep.Step
	}{
		{"sent untracked", refuseTracking, func(email automationstep.Step, b openedBranch) []automationstep.Step {
			return []automationstep.Step{email, b.step}
		}},
		{"sent untracked by the Workspace's dont_track default", dontTrack, func(email automationstep.Step, b openedBranch) []automationstep.Step {
			return []automationstep.Step{email, b.step}
		}},
		{"skipped", func(t *testing.T, env *testhelper.TestEnv) {
			t.Helper()
			env.SuppressDestination(t.Context(), fixtures.ContactAliceEmail)
		},
			func(email automationstep.Step, b openedBranch) []automationstep.Step {
				return []automationstep.Step{email, b.step}
			}},
		{"never passed", func(*testing.T, *testhelper.TestEnv) {}, func(email automationstep.Step, b openedBranch) []automationstep.Step {
			earlier := automationstep.Step{ID: automationstep.NewID(), Payload: automationstep.Branch{
				Paths:     []automationstep.BranchPath{pathOf(automationstep.NewID(), condOf(t, hasTag("never-given")), email)},
				Otherwise: plainPath(automationstep.NewID()),
			}}
			return []automationstep.Step{earlier, b.step}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			email := welcomeEmail()
			b := branchOnOpen(t, email.ID)
			auto := publishedBranching(t, env, fixtures.AcmeID, tc.steps(email, b)...)
			tc.before(t, env)
			runID := enrollIn(t, env, fixtures.AcmeID, auto.ID, fixtures.ContactAliceID)

			env.RunQueuedSteps(t, stepperOf(env, resolvingTo(&fakeSender{})))

			rec := recordOf(t, env, runID, b.step.ID)
			took := tookPath(rec)
			assert.Equal(t, b.unknown, took)
			assert.Equal(t, map[automationstep.ID]conditiontree.Truth{b.opened: conditiontree.TruthUnknown}, rec.PathOutcomes)
		})
	}
}
