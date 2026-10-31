package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// enrollable is the nth Contact of the Workspace, by id, that no Automation has enrolled yet.
func enrollable(t *testing.T, s *ent.Scoped, nth int) *ent.Contact {
	t.Helper()
	enrolledIDs, err := s.AutomationRun().Query().Select(automationrun.FieldContactID).Ints(context.Background())
	require.NoError(t, err)
	enrolled := lo.Map(enrolledIDs, func(id int, _ int) int64 { return int64(id) })
	free, err := s.Contact().Query().
		Where(contact.EmailNotNil(), contact.IDNotIn(enrolled...)).
		Order(ent.Asc(contact.FieldID)).Limit(nth + 1).All(context.Background())
	require.NoError(t, err)
	require.Greater(t, len(free), nth, "the fixtures hold enough Contacts no Automation has enrolled")
	return free[nth]
}

func TestEnrollIsOnceEverPerContactAndAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)

	runID, enrolled, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	require.True(t, enrolled)
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)

	_, again, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "a replayed trigger enrolls nothing")

	_, other, err := automations.Enroll(ctx, s, fixtures.AutomationHoldDemoID, c.ID)
	require.NoError(t, err)
	assert.True(t, other, "another Automation is a separate Enrollment")
}

func TestExitEndsOnlyAnActiveEnrollmentAndEnrollmentStaysOnceEver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)

	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)

	exited, err := automations.Exit(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.True(t, exited)
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Nil(t, run.ResumeAt)

	again, err := automations.Exit(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "nothing active is left to exit")

	_, reenrolled, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, reenrolled, "an exited Contact is not enrolled again")
}

func TestExitRunEndsOneEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)

	require.NoError(t, automations.ExitRun(ctx, s, runID))
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
}

func TestAnExitedEnrollmentIsNeitherCompletedNorFailed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	require.NoError(t, automations.ExitRun(ctx, s, runID))

	require.ErrorIs(t, automations.Complete(ctx, s, runID), lifecycle.ErrInvalidTransition)
	require.ErrorIs(t, automations.Fail(ctx, s, runID, automations.Failure{}), lifecycle.ErrInvalidTransition)
	require.ErrorIs(t, automations.ExitRun(ctx, s, runID), lifecycle.ErrInvalidTransition)
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
}

func TestFailKeepsTheReasonAndCompleteEndsTheRun(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	failing, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, enrollable(t, s, 0).ID)
	require.NoError(t, err)
	done, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, enrollable(t, s, 1).ID)
	require.NoError(t, err)

	require.NoError(t, automations.Fail(ctx, s, failing, automations.Failure{
		Kind: automationrun.FailureKindHTTPStatus, Status: 500, Excerpt: "boom",
	}))
	require.NoError(t, automations.Complete(ctx, s, done))

	run, err := s.AutomationRun().Get(ctx, failing)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusFailed, run.Status)
	assert.Equal(t, automationrun.FailureKindHTTPStatus, *run.FailureKind)
	assert.Equal(t, 500, *run.FailureStatus)
	assert.Equal(t, "boom", *run.FailureExcerpt)
	run, err = s.AutomationRun().Get(ctx, done)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusCompleted, run.Status)
}

func TestEnrollRefusesAContactOrAutomationOfAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	globex := env.DB.Scoped(fixtures.GlobexID)
	c := enrollable(t, acme, 0)

	_, _, err := automations.Enroll(ctx, globex, fixtures.AutomationGlobexID, c.ID)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace, "Globex cannot enroll an Acme Contact")
	_, _, err = automations.Enroll(ctx, acme, fixtures.AutomationGlobexID, c.ID)
	assert.ErrorIs(t, err, ent.ErrNotInWorkspace, "Acme cannot enroll into a Globex Automation")
}

func TestAdvanceMovesOnlyAnActiveEnrollmentStandingOnTheExpectedStep(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	live, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, enrollable(t, s, 0).ID)
	require.NoError(t, err)
	gone, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, enrollable(t, s, 1).ID)
	require.NoError(t, err)
	require.NoError(t, automations.ExitRun(ctx, s, gone))
	resume := time.Now().Add(time.Hour)

	require.ErrorIs(t, automations.Advance(ctx, s, live, 5, 6, &resume), lifecycle.ErrInvalidTransition, "an Enrollment elsewhere than the expected step stays put")
	require.NoError(t, automations.Advance(ctx, s, live, 0, 1, &resume))
	require.ErrorIs(t, automations.Advance(ctx, s, live, 0, 1, &resume), lifecycle.ErrInvalidTransition, "the same move cannot be made twice")
	require.ErrorIs(t, automations.Advance(ctx, s, gone, 0, 1, &resume), lifecycle.ErrInvalidTransition)

	run, err := s.AutomationRun().Get(ctx, live)
	require.NoError(t, err)
	assert.Equal(t, 1, run.CurrentStep)
	require.NotNil(t, run.ResumeAt)
	run, err = s.AutomationRun().Get(ctx, gone)
	require.NoError(t, err)
	assert.Equal(t, 0, run.CurrentStep)
	assert.Nil(t, run.ResumeAt)
}

func TestIsActiveReadsTheLifecycle(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, enrollable(t, s, 0).ID)
	require.NoError(t, err)

	ok, err := automations.IsActive(ctx, s, runID)
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, automations.ExitRun(ctx, s, runID))
	ok, err = automations.IsActive(ctx, s, runID)
	require.NoError(t, err)
	assert.False(t, ok)
}

// The trigger consumer picks the Automations before it enrolls, so a Publish that moves the
// Trigger, or a Deactivate, can land in between: under the lock, an Automation that is no longer
// active, or whose live version starts on another Event, enrolls nobody.
func TestEnrollOnTriggerEnrollsOnlyWhileActiveOnThatTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{TriggerEvent: new("hold_demo_moved")})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)

	_, enrolled, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID, automations.Origin{Action: "hold_demo", OccurredAt: time.Now()})
	require.NoError(t, err)
	assert.False(t, enrolled, "the Trigger it was picked by is no longer live")

	_, err = m.Deactivate(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	_, enrolled, err = automations.EnrollOnTrigger(ctx, s, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID, automations.Origin{Action: "hold_demo_moved", OccurredAt: time.Now()})
	require.NoError(t, err)
	assert.False(t, enrolled, "an inactive Automation enrolls nobody")

	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	_, enrolled, err = automations.EnrollOnTrigger(ctx, s, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID, automations.Origin{Action: "hold_demo_moved", OccurredAt: time.Now()})
	require.NoError(t, err)
	assert.True(t, enrolled, "active on that Trigger, it enrolls the Contact the refusals left alone")
}
