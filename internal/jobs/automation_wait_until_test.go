package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The fixture Automation AutomationJobsWaitUntil waits until the next weekday at 09:00 in the
// Contact's zone, then sends an email (ADR 0126). Its moments are read on 15 January 2027, a
// Friday, away from any daylight-saving change.

// harnessClock is an Instance clock a test moves: the env reads it, and so does the Step execution
// the test builds on it.
type harnessClock struct{ now time.Time }

func (c *harnessClock) read() time.Time { return c.now }

// waitUntilEnv is the env on a clock at the moment given, and that clock.
func waitUntilEnv(t *testing.T, at time.Time) (*testhelper.TestEnv, *harnessClock) {
	t.Helper()
	clock := &harnessClock{now: at}
	return testhelper.Setup(t, testhelper.WithClock(clock.read)), clock
}

// enrollInWaitUntil enrolls the Contact in AutomationJobsWaitUntil on its Trigger, the Event
// occurring at the clock's moment, and returns its one Enrollment.
func enrollInWaitUntil(t *testing.T, env *testhelper.TestEnv, clock *harnessClock, contactID int64) int64 {
	t.Helper()
	err := testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{
		ContactID: contactID,
		Event:     automations.Event{Action: triggerOf(t, env, fixtures.AutomationJobsWaitUntilID), OccurredAt: clock.now},
	})
	require.NoError(t, err)
	runIDs := enrollmentsOf(t, env, fixtures.AutomationJobsWaitUntilID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

func zoneOf(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	require.NoError(t, err)
	return loc
}

// Wait until "the next weekday at 9:00", reached on Friday at 18:00 in the Contact's New York,
// stands on the Step until Monday at 9:00 there: its due moment is on the Enrollment and in the
// Step record, and its one job is queued for then. A restart that runs the job before then neither
// moves the Enrollment nor queues the job twice; at that moment the next Step runs.
func TestAWaitUntilTheNextWeekdayStandsOnItUntilMondayInTheContactsZone(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 18, 0, 0, 0, newYork))
	ctx := t.Context()
	monday := time.Date(2027, time.January, 18, 9, 0, 0, 0, newYork)
	fs := &fakeSender{}
	stepper := stepperAt(env, newMod(env, resolvingTo(fs), clock.read), clock.read)
	runID := enrollInWaitUntil(t, env, clock, fixtures.ContactLiamID)

	reached := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	require.NotNil(t, reached.ResumeAt)
	require.Len(t, records, 1)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(reached.CurrentStepID), "the Enrollment stands on the Wait until")
	assert.WithinDuration(t, monday, *reached.ResumeAt, 0, "due on Monday at 9:00 in New York")
	assert.Equal(t, reached.ResumeAt, records[0].DueAt, "the Step record keeps the due moment")
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "its own job is queued, and nothing else")
	assert.WithinDuration(t, monday, env.JobSchedules(t, automations.RunStepArgs{}.Kind())[0], time.Millisecond, "for the due moment")

	clock.now = time.Date(2027, time.January, 16, 12, 0, 0, 0, newYork) // a restart on Saturday
	require.True(t, env.RunNextStep(t, stepper))

	restarted := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Empty(t, fs.sent, "the email after the Wait until is not sent early")
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(restarted.CurrentStepID), "the Enrollment stays on the Wait until")
	assert.Equal(t, reached.ResumeAt, restarted.ResumeAt, "the due moment is not computed again")
	assert.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env), "the job is queued once")

	clock.now = monday
	env.RunQueuedSteps(t, stepper)

	ended := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Len(t, fs.sent, 1, "at the due moment the next Step sends")
	assert.Equal(t, automationrun.StatusCompleted, ended.Status)
}

// A Contact with no time zone waits for 9:00 in the Workspace time zone.
func TestAWaitUntilOfAContactWithNoZoneReadsTheWorkspaceTimeZone(t *testing.T) {
	berlin := zoneOf(t, "Europe/Berlin")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 18, 0, 0, 0, berlin))
	ctx := t.Context()
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetTimezone("Europe/Berlin").ExecX(ctx)

	runID := enrollInWaitUntil(t, env, clock, fixtures.ContactAliceID)

	run := env.DB.AutomationRun.GetX(ctx, runID)
	require.NotNil(t, run.ResumeAt)
	assert.WithinDuration(t, time.Date(2027, time.January, 18, 9, 0, 0, 0, berlin), *run.ResumeAt, 0, "Monday at 9:00 in Berlin")
}

// The due moment is computed once: a Contact's time zone changed during the wait does not move it,
// and the Enrollment goes on at the moment it was due in the zone it had.
func TestAContactsTimeZoneChangedDuringAWaitUntilDoesNotMoveIt(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 18, 0, 0, 0, newYork))
	ctx := t.Context()
	fs := &fakeSender{}
	stepper := stepperAt(env, newMod(env, resolvingTo(fs), clock.read), clock.read)
	runID := enrollInWaitUntil(t, env, clock, fixtures.ContactLiamID)
	due := env.DB.AutomationRun.GetX(ctx, runID).ResumeAt

	env.DB.Contact.UpdateOneID(fixtures.ContactLiamID).SetTimeZone("Asia/Tokyo").ExecX(ctx)
	clock.now = time.Date(2027, time.January, 18, 9, 0, 0, 0, newYork)
	env.RunQueuedSteps(t, stepper)

	records := stepRecordsOf(t, env, runID)
	require.NotEmpty(t, records)
	assert.Equal(t, due, records[0].DueAt, "the due moment stayed as it was computed")
	assert.Len(t, fs.sent, 1, "the Enrollment went on at Monday 9:00 in New York")
}

// An exit Event recorded during a Wait until ends the Enrollment there, as anywhere else.
func TestAnExitEventDuringAWaitUntilEndsTheEnrollmentThere(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 18, 0, 0, 0, newYork))
	ctx := t.Context()
	runID := enrollInWaitUntil(t, env, clock, fixtures.ContactLiamID)
	clock.now = time.Date(2027, time.January, 16, 10, 0, 0, 0, newYork)

	err := testhelper.Module[*automations.StepExecution](env).OnEvent(ctx, env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{
		ContactID: fixtures.ContactLiamID,
		Event:     automations.Event{Action: "job_wait_until_left", OccurredAt: clock.now},
	})
	require.NoError(t, err)

	run := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(run.CurrentStepID), "it ended on the Wait until")
	require.Len(t, records, 1)
	assert.NotNil(t, records[0].LeftAt, "the Wait until was left as the Enrollment ended")
}

// Erasing the Contact during a Wait until removes its queued job with it.
func TestErasingTheContactDuringAWaitUntilRemovesTheQueuedJob(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 18, 0, 0, 0, newYork))
	runID := enrollInWaitUntil(t, env, clock, fixtures.ContactLiamID)
	require.Equal(t, []map[string]any{stepJob(t, env, runID, 0)}, queuedSteps(t, env))

	err := env.ExternalScoped(t, apitokens.ScopeContactsErase).ContactsDelete(t.Context(), externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactLiamID)})
	require.NoError(t, err)

	assert.Empty(t, queuedSteps(t, env), "the Wait until's job went with the Contact")
}
