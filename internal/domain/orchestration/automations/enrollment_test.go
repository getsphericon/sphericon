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
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
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

// enrollNow enrolls the Contact in the Automation as a Rule whose Event is no longer stored does:
// the Enrollment begins now and no Event began it. It reports whether one was made.
func enrollNow(t *testing.T, env *testhelper.TestEnv, s *ent.Scoped, automationID, contactID int64) (bool, error) {
	t.Helper()
	return env.StepExecution.EnrollOnRule(context.Background(), s, automations.EnrollOnRuleInput{AutomationID: automationID, ContactID: contactID})
}

// runOf is the Contact's one Enrollment in the Automation.
func runOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) int64 {
	t.Helper()
	runs := runsOf(t, env, automationID, contactID)
	require.Len(t, runs, 1)
	return runs[0].ID
}

func TestEnrollIsOnceEverPerContactAndAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)

	enrolled, err := enrollNow(t, env, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	require.True(t, enrolled)
	run, err := s.AutomationRun().Get(ctx, runOf(t, env, fixtures.AutomationWelcomeSeriesID, c.ID))
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, run.Status)

	again, err := enrollNow(t, env, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, again, "a replayed trigger enrolls nothing")

	other, err := enrollNow(t, env, s, fixtures.AutomationHoldDemoID, c.ID)
	require.NoError(t, err)
	assert.True(t, other, "another Automation is a separate Enrollment")
}

func TestExitEndsOnlyAnActiveEnrollmentAndEnrollmentStaysOnceEver(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)

	_, err := enrollNow(t, env, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	runID := runOf(t, env, fixtures.AutomationWelcomeSeriesID, c.ID)

	exited, err := automations.Exit(ctx, s, env.Publisher(), fixtures.AutomationWelcomeSeriesID, c.ID, time.Now())
	require.NoError(t, err)
	assert.True(t, exited)
	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Nil(t, run.ResumeAt)

	again, err := automations.Exit(ctx, s, env.Publisher(), fixtures.AutomationWelcomeSeriesID, c.ID, time.Now())
	require.NoError(t, err)
	assert.False(t, again, "nothing active is left to exit")

	reenrolled, err := enrollNow(t, env, s, fixtures.AutomationWelcomeSeriesID, c.ID)
	require.NoError(t, err)
	assert.False(t, reenrolled, "an exited Contact is not enrolled again")
}

func TestExitEndsOneEnrollmentOfTheAutomation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	waiting, err := s.AutomationRun().Get(ctx, fixtures.AutomationRunWelcomeWaitingID)
	require.NoError(t, err)

	exited, err := automations.Exit(ctx, s, env.Publisher(), fixtures.AutomationWelcomeSeriesID, waiting.ContactID, time.Now())
	require.NoError(t, err)

	assert.True(t, exited)
	run, err := s.AutomationRun().Get(ctx, fixtures.AutomationRunWelcomeWaitingID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	other, err := s.AutomationRun().Get(ctx, fixtures.AutomationRunWelcomeWaitingLaterID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, other.Status, "another Enrollment of the Automation goes on")
}

// A step job for an Enrollment that has left `active` neither completes nor fails it, and queues
// nothing: the lifecycle table decides that it can no longer move.
func TestAStepOfAnExitedEnrollmentLeavesItExited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID := int64(fixtures.AutomationRunWelcomeWaitingLaterID)
	waiting, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	_, err = automations.Exit(ctx, s, env.Publisher(), fixtures.AutomationWelcomeSeriesID, waiting.ContactID, time.Now())
	require.NoError(t, err)

	require.NoError(t, env.StepExecution.Step(ctx, s, runID, waiting.CurrentStep))
	require.NoError(t, env.StepExecution.Step(ctx, s, runID, waiting.CurrentStep+1))

	run, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, waiting.CurrentStep, run.CurrentStep)
	assert.Nil(t, run.FailureKind)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

func TestEnrollRefusesAContactOrAutomationOfAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	globex := env.DB.Scoped(fixtures.GlobexID)
	c := enrollable(t, acme, 0)

	_, err := enrollNow(t, env, globex, fixtures.AutomationGlobexID, c.ID)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace, "Globex cannot enroll an Acme Contact")
	_, err = enrollNow(t, env, acme, fixtures.AutomationGlobexID, c.ID)
	assert.ErrorIs(t, err, ent.ErrNotInWorkspace, "Acme cannot enroll into a Globex Automation")
}

// A step job that expects the Enrollment elsewhere than it stands (delivered twice, or after a
// competing writer moved it) leaves it where it is and queues nothing.
func TestAStepJobForAnotherStepLeavesTheEnrollmentWhereItStands(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID := int64(fixtures.AutomationRunWelcomeWaitingID)
	before, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)

	require.NoError(t, env.StepExecution.Step(ctx, s, runID, before.CurrentStep+4))

	after, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusActive, after.Status)
	assert.Equal(t, before.CurrentStep, after.CurrentStep)
	assert.Equal(t, before.ResumeAt, after.ResumeAt)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued)
}

// An Event enrolls the Contact into an Automation only while it is active and its live version
// starts on that Event's action: a Publish that moves the Trigger, or a Deactivate, ends it.
func TestAnEventEnrollsOnlyWhileTheAutomationIsActiveOnThatTrigger(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{TriggerEvent: new("hold_demo_moved")})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)

	handEvent(t, env, fixtures.ContactAliceID, "hold_demo", time.Now())
	afterTheOldTrigger := runsOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	_, err = m.Deactivate(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	handEvent(t, env, fixtures.ContactAliceID, "hold_demo_moved", time.Now())
	whileInactive := runsOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	handEvent(t, env, fixtures.ContactAliceID, "hold_demo_moved", time.Now())
	onItsTrigger := runsOf(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)

	assert.Empty(t, afterTheOldTrigger, "the Trigger it was picked by is no longer live")
	assert.Empty(t, whileInactive, "an inactive Automation enrolls nobody")
	assert.Len(t, onItsTrigger, 1, "active on that Trigger, it enrolls the Contact the refusals left alone")
}
