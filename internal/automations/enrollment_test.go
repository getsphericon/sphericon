package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/lifecycle"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func newContact(t *testing.T, s *ent.Scoped, email string) *ent.Contact {
	t.Helper()
	c, err := s.Contact().Create().SetEmail(email).Save(context.Background())
	require.NoError(t, err)
	return c
}

func TestEnrollIsOnceEverPerContactAndAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := newContact(t, s, "enroll-once@example.com")

	runID, enrolled, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	require.True(t, enrolled)
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)

	_, again, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "a replayed trigger enrolls nothing")

	_, other, err := automations.Enroll(ctx, s, fixtures.AutomationWinBackID, c.ID)
	require.NoError(t, err)
	assert.True(t, other, "another Automation is a separate Enrollment")
}

func TestExitEndsOnlyAnActiveEnrollmentAndEnrollmentStaysOnceEver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := newContact(t, s, "exit@example.com")

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
	c := newContact(t, s, "exit-run@example.com")
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
	c := newContact(t, s, "late-step@example.com")
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
	failing, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, newContact(t, s, "fail@example.com").ID)
	require.NoError(t, err)
	done, _, err := automations.Enroll(ctx, s, fixtures.AutomationWelcomeSeriesID, newContact(t, s, "done@example.com").ID)
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
	c := newContact(t, acme, "isolated@example.com")

	_, _, err := automations.Enroll(ctx, globex, fixtures.AutomationGlobexID, c.ID)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace, "Globex cannot enroll an Acme Contact")
	_, _, err = automations.Enroll(ctx, acme, fixtures.AutomationGlobexID, c.ID)
	assert.ErrorIs(t, err, ent.ErrNotInWorkspace, "Acme cannot enroll into a Globex Automation")
}
