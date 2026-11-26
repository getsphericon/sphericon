package jobs_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationsteprecord"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The fixture Automation AutomationJobsWaitUntilDate waits until 3 days before the Contact's
// trial_ends_at at 10:00, and AutomationJobsWaitUntilBirthday until its next birthday at 10:00,
// each in the Contact's zone, then sends an email (ADR 0126). Liam lives in New York; the
// Enrollments arrive on Friday 15 January 2027 at noon there.

// enrollOnItsTrigger enrolls the Contact in the fixture Automation on its Trigger, the Event
// occurring at the clock's moment, and returns its one Enrollment.
func enrollOnItsTrigger(t *testing.T, env *testhelper.TestEnv, clock *harnessClock, automationID, contactID int64) int64 {
	t.Helper()
	err := testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), automations.OnEventInput{
		ContactID: contactID,
		Event:     automations.Event{Action: triggerOf(t, env, automationID), OccurredAt: clock.now},
	})
	require.NoError(t, err)
	runIDs := enrollmentsOf(t, env, automationID, contactID)
	require.Len(t, runIDs, 1)
	return runIDs[0]
}

// "3 days before trial_ends_at at 10:00" with the field ahead stands on the Step until that moment
// in the Contact's zone, then sends.
func TestAWaitUntilADateAheadStandsOnItUntilItsMoment(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 12, 0, 0, 0, newYork))
	ctx := t.Context()
	env.DB.Contact.UpdateOneID(fixtures.ContactLiamID).SetCustomFields(map[string]any{"trial_ends_at": "2027-01-20"}).ExecX(ctx)
	fs := &fakeSender{}
	stepper := stepperAt(env, newMod(env, resolvingTo(fs), clock.read), clock.read)
	sunday := time.Date(2027, time.January, 17, 10, 0, 0, 0, newYork)

	runID := enrollOnItsTrigger(t, env, clock, fixtures.AutomationJobsWaitUntilDateID, fixtures.ContactLiamID)

	waiting := env.DB.AutomationRun.GetX(ctx, runID)
	records := stepRecordsOf(t, env, runID)
	require.NotNil(t, waiting.ResumeAt)
	require.Len(t, records, 1)
	assert.Equal(t, env.StepIDAt(t, runID, 0), lo.FromPtr(waiting.CurrentStepID), "the Enrollment stands on the Wait until")
	assert.WithinDuration(t, sunday, *waiting.ResumeAt, 0, "due 3 days before the trial ends, at 10:00 in New York")
	assert.Nil(t, records[0].WentOn)
	assert.Empty(t, fs.sent)

	clock.now = sunday
	env.RunQueuedSteps(t, stepper)

	ended := env.DB.AutomationRun.GetX(ctx, runID)
	assert.Equal(t, automationrun.StatusCompleted, ended.Status)
	assert.Len(t, fs.sent, 1, "at its moment the email after the Wait until is sent")
}

// With no moment ahead within 366 days the Enrollment goes on at once: the email after the Wait
// until is sent on arrival, and the Step record keeps why, with no due moment.
func TestAWaitUntilADateItCannotReachGoesOnAtOnceWithItsReason(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields map[string]any
		want   automationstep.WentOn
	}{
		{"the field in the past", map[string]any{"trial_ends_at": "2027-01-10"}, automationstep.WentOnPast},
		{"no field", map[string]any{}, automationstep.WentOnNoDate},
		{"a field that holds no date", map[string]any{"trial_ends_at": "soon"}, automationstep.WentOnNoDate},
		{"the field two years ahead", map[string]any{"trial_ends_at": "2029-01-20"}, automationstep.WentOnBeyondBound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newYork := zoneOf(t, "America/New_York")
			env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 12, 0, 0, 0, newYork))
			ctx := t.Context()
			env.DB.Contact.UpdateOneID(fixtures.ContactLiamID).SetCustomFields(tc.fields).ExecX(ctx)
			fs := &fakeSender{}
			stepper := stepperAt(env, newMod(env, resolvingTo(fs), clock.read), clock.read)
			runID := enrollOnItsTrigger(t, env, clock, fixtures.AutomationJobsWaitUntilDateID, fixtures.ContactLiamID)

			env.RunQueuedSteps(t, stepper)

			ended := env.DB.AutomationRun.GetX(ctx, runID)
			records := stepRecordsOf(t, env, runID)
			require.Len(t, records, 2)
			assert.Equal(t, automationrun.StatusCompleted, ended.Status)
			assert.Len(t, fs.sent, 1, "the email after the Wait until is sent at once")
			assert.Equal(t, &tc.want, records[0].WentOn)
			assert.Nil(t, records[0].DueAt, "no moment to wait for")
			assert.Equal(t, lo.ToPtr(automationsteprecord.OutcomeDone), records[0].Outcome)
			assert.Equal(t, records[0].ReachedAt, *records[0].LeftAt, "left as it was reached")
		})
	}
}

// "Until the next birthday at 10:00" waits for this year's birthday when it is ahead, and for next
// year's when it has passed.
func TestAWaitUntilTheNextBirthdayWaitsForThisYearsOrNextYears(t *testing.T) {
	newYork := zoneOf(t, "America/New_York")
	for _, tc := range []struct {
		name     string
		birthday time.Time
		want     time.Time
	}{
		{"this year's, ahead", time.Date(1990, time.March, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, time.March, 1, 10, 0, 0, 0, newYork)},
		{"next year's, this year's passed", time.Date(1990, time.January, 10, 0, 0, 0, 0, time.UTC), time.Date(2028, time.January, 10, 10, 0, 0, 0, newYork)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, clock := waitUntilEnv(t, time.Date(2027, time.January, 15, 12, 0, 0, 0, newYork))
			ctx := t.Context()
			env.DB.Contact.UpdateOneID(fixtures.ContactLiamID).SetBirthday(tc.birthday).ExecX(ctx)

			runID := enrollOnItsTrigger(t, env, clock, fixtures.AutomationJobsWaitUntilBirthdayID, fixtures.ContactLiamID)

			run := env.DB.AutomationRun.GetX(ctx, runID)
			require.NotNil(t, run.ResumeAt)
			assert.WithinDuration(t, tc.want, *run.ResumeAt, 0)
		})
	}
}
