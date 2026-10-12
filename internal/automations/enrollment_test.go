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

func TestEnrollRefusesAContactOrAutomationOfAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	globex := env.DB.Scoped(fixtures.GlobexID)
	c := newContact(t, acme, "isolated@example.com")

	_, _, err := automations.Enroll(ctx, globex, fixtures.AutomationGlobexID, c.ID)
	assert.ErrorIs(t, err, ent.ErrNotInWorkspace, "Globex cannot enroll an Acme Contact")
	_, _, err = automations.Enroll(ctx, acme, fixtures.AutomationGlobexID, c.ID)
	assert.ErrorIs(t, err, ent.ErrNotInWorkspace, "Acme cannot enroll into a Globex Automation")
}
