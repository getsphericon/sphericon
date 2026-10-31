package automations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Payment recovery fixture's live version: trigger payment_failed, exit events account_closed,
// plan_downgraded and email.clicked, goal event payment_succeeded (#751).
const (
	recoveryExit = "account_closed"
	recoveryGoal = "payment_succeeded"
)

// statusOf is the status an Enrollment is in now.
func statusOf(t *testing.T, env *testhelper.TestEnv, runID int64) automationrun.Status {
	t.Helper()
	run, err := env.DB.AutomationRun.Get(context.Background(), runID)
	require.NoError(t, err)
	return run.Status
}

func TestTheGoalEventEndsTheEnrollmentAsGoalReached(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, time.Now()))

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusGoalReached, status)
	active, err := automations.IsActive(ctx, s, runID)
	require.NoError(t, err)
	assert.False(t, active, "no later step runs")
}

func TestAnExitEventEndsTheEnrollmentAsExited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryExit, time.Now()))
	exited := statusOf(t, env, runID)
	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, time.Now()))
	afterTheGoal := statusOf(t, env, runID)

	assert.Equal(t, automationrun.StatusExited, exited)
	assert.Equal(t, automationrun.StatusExited, afterTheGoal, "an ended Enrollment stays as it ended")
}

// An Enrollment begins when its triggering Event occurred: history from before then, loaded or
// sent late, does not end it.
func TestAnEventThatOccurredBeforeTheEnrollmentBeganEndsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	triggered := time.Now().Add(-time.Hour)
	runID, _, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID, automations.Origin{Action: "payment_failed", OccurredAt: triggered})
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, triggered.Add(-time.Second)))
	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryExit, triggered.Add(-time.Second)))

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusActive, status)
}

// The goal Event occurred after the trigger but before the Enrollment was made (its job ran
// late): it still ends the Enrollment, which began when the trigger occurred, not when the run
// was written.
func TestAnEventAfterTheTriggerEndsAnEnrollmentMadeAfterIt(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	triggered := time.Now().Add(-2 * time.Minute)
	paid := triggered.Add(time.Minute)
	runID, enrolled, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID, automations.Origin{Action: "payment_failed", OccurredAt: triggered})
	require.NoError(t, err)
	require.True(t, enrolled)
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	paidBeforeWritten := paid.Before(run.CreatedAt)
	require.True(t, paidBeforeWritten, "the payment occurred before the run was written")

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, paid))

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusGoalReached, status)
}

// An Enrollment an exit or goal Event ends, ends when that Event occurred, on the clock its
// beginning was read on, not when the Event was handled (#753).
func TestAnEnrollmentEndsWhenItsEndingEventOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	triggered := time.Now().Add(-2 * time.Hour)
	paid := triggered.Add(time.Hour)
	runID, _, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID, automations.Origin{Action: "payment_failed", OccurredAt: triggered})
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, paid))

	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	require.NotNil(t, run.EndedAt)
	assert.WithinDuration(t, paid, *run.EndedAt, time.Microsecond)
}

// An Enrollment made already ended ends when the first exit or goal Event since its trigger
// occurred (fixture: account_closed 5 days ago for ContactPaidBeforeEnrolled).
func TestAnEnrollmentMadeEndedEndsWhenThatEventOccurred(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	closed, err := env.DB.Event.Query().
		Where(event.ContactID(fixtures.ContactPaidBeforeEnrolledID), event.Action(recoveryExit)).
		Only(ctx)
	require.NoError(t, err)

	runID, _, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID,
		fixtures.ContactPaidBeforeEnrolledID, automations.Origin{Action: "payment_failed", OccurredAt: time.Now().Add(-6 * 24 * time.Hour)})
	require.NoError(t, err)

	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, &closed.OccurredAt, run.EndedAt)
}

// A step ends an Enrollment at the moment its caller's clock gives.
func TestAStepEndsAnEnrollmentAtTheCallersClock(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	at := time.Date(2026, 3, 12, 9, 30, 0, 0, time.UTC)

	require.NoError(t, automations.Complete(ctx, s, fixtures.AutomationRunTagOnEngagementAliceID, at))

	run, err := env.DB.AutomationRun.Get(ctx, fixtures.AutomationRunTagOnEngagementAliceID)
	require.NoError(t, err)
	require.NotNil(t, run.EndedAt)
	assert.True(t, at.Equal(*run.EndedAt), "ended at %s", run.EndedAt)
}

// The exit or goal Event was handled before the Enrollment was made, with nothing to end: the
// Enrollment is made already ended by the first such Event recorded since the trigger, and no step
// is to run. A machine click and a backfilled Event end nothing (ADR 0053). The Contact's Events
// (fixture): account_closed 5 days ago, payment_succeeded 3 days ago, a machine email.clicked and a
// backfilled plan_downgraded a day ago.
func TestAnEnrollmentIsMadeEndedByAnExitOrGoalEventRecordedSinceTheTrigger(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name         string
		triggered    time.Duration
		wantStatus   automationrun.Status
		wantEnrolled bool
	}{
		{"the first since the trigger is an exit", 6 * day, automationrun.StatusExited, false},
		{"the first since the trigger is the goal", 4 * day, automationrun.StatusGoalReached, false},
		{"only a machine click and loaded history since", 2 * day, automationrun.StatusActive, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			s := env.DB.Scoped(fixtures.AcmeID)

			runID, enrolled, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID,
				fixtures.ContactPaidBeforeEnrolledID, automations.Origin{Action: "payment_failed", OccurredAt: time.Now().Add(-tc.triggered)})
			require.NoError(t, err)
			status := statusOf(t, env, runID)
			againID, _, err := automations.EnrollOnTrigger(ctx, s, fixtures.AutomationPaymentRecoveryID,
				fixtures.ContactPaidBeforeEnrolledID, automations.Origin{Action: "payment_failed", OccurredAt: time.Now()})
			require.NoError(t, err)

			assert.Equal(t, tc.wantEnrolled, enrolled, "a step is to run")
			assert.Equal(t, tc.wantStatus, status)
			assert.Zero(t, againID, "enrolled once ever: no second Enrollment is made")
		})
	}
}

// Matching is by Contact and by the actions the version lists, nothing else.
func TestAnUnlistedEventOrAnotherContactsEventEndsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c, other := enrollable(t, s, 0), enrollable(t, s, 1)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, "payment_failed", time.Now()))
	require.NoError(t, automations.EndOnEvent(ctx, s, other.ID, recoveryGoal, time.Now()))

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusActive, status)
}

// An Enrollment reads its exit and goal events from the version it stands on: a later Publish
// that changes them, or a Deactivate, does not change what ends it.
func TestAnEnrollmentEndsByTheEventsOfTheVersionItStandsOn(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	runID, _, err := automations.Enroll(ctx, s, fixtures.AutomationPaymentRecoveryID, c.ID)
	require.NoError(t, err)

	_, err = m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{
		ExitEvents: &[]string{"card_removed"}, GoalEvent: patch.Clear[string](),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, fixtures.AutomationPaymentRecoveryID)
	require.NoError(t, err)
	_, err = m.Deactivate(ctx, s, fixtures.AutomationPaymentRecoveryID)
	require.NoError(t, err)

	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, "card_removed", time.Now()))
	afterTheNewExit := statusOf(t, env, runID)
	require.NoError(t, automations.EndOnEvent(ctx, s, c.ID, recoveryGoal, time.Now()))
	afterItsGoal := statusOf(t, env, runID)

	assert.Equal(t, automationrun.StatusActive, afterTheNewExit, "the newer version's exit is not this Enrollment's")
	assert.Equal(t, automationrun.StatusGoalReached, afterItsGoal)
}

// The Event a Rule fired on is no longer stored (Retention, an Erasure): the Enrollment begins
// now, so the Contact's older history does not end it.
func TestARuleEnrollmentWithoutItsStoredEventBeginsNow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	earliest := time.Now().Add(-time.Microsecond) // began_at is kept to the microsecond

	runID, enrolled, err := automations.EnrollOnRule(ctx, s, fixtures.AutomationPaymentRecoveryID,
		fixtures.ContactPaidBeforeEnrolledID, "evt-not-stored")
	require.NoError(t, err)
	latest := time.Now()
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)

	assert.True(t, enrolled)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.WithinRange(t, run.BeganAt, earliest, latest)
}

func TestCreateKeepsExitAndGoalEventsInTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	goal := " subscription_expanded "

	created, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Upsell", TriggerEvent: "plan_limit_reached",
		ExitEvents: []string{"account_closed", " account_closed", "email.unsubscribed"}, GoalEvent: &goal,
	})
	require.NoError(t, err)

	_, draft := automations.LiveAndDraft(created)
	require.NotNil(t, draft)
	assert.Equal(t, []string{"account_closed", "email.unsubscribed"}, draft.ExitEvents, "trimmed, a repeat kept once")
	assert.Equal(t, new("subscription_expanded"), draft.GoalEvent)
}

func TestCreateRefusesAnExitOrGoalEventThatCannotEndAnEnrollment(t *testing.T) {
	cases := []struct {
		name string
		in   automations.CreateInput
		want error
	}{
		{"an exit equal to the trigger", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{"payment_failed"}}, automations.ErrInvalidExitEvent},
		{"an empty exit", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{" "}}, automations.ErrInvalidExitEvent},
		{"an infrastructure exit", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{"email.sent"}}, automations.ErrInvalidExitEvent},
		{"a goal equal to the trigger", automations.CreateInput{TriggerEvent: "payment_failed", GoalEvent: new("payment_failed")}, automations.ErrInvalidGoalEvent},
		{"an infrastructure goal", automations.CreateInput{TriggerEvent: "payment_failed", GoalEvent: new("email.delivered")}, automations.ErrInvalidGoalEvent},
		{"a goal that is also an exit", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{"paid"}, GoalEvent: new("paid")}, automations.ErrInvalidGoalEvent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
			tc.in.Name = "Refused"

			_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), tc.in)

			require.ErrorIs(t, err, tc.want)
		})
	}
}

// The edit is checked against the definition it makes: a new trigger equal to a stored exit
// event is refused, though the edit names no exit event.
func TestUpdateRefusesATriggerEqualToAStoredExitEvent(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{
		TriggerEvent: new(recoveryExit),
	})

	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
}

// Exit and goal events are the definition (ADR 0065): editing them writes the draft and leaves the
// live version as it was.
func TestEditingExitAndGoalEventsWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	updated, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{
		ExitEvents: &[]string{}, GoalEvent: patch.Set("invoice_paid"),
	})
	require.NoError(t, err)

	live, draft := automations.LiveAndDraft(updated)
	require.NotNil(t, draft)
	assert.Empty(t, draft.ExitEvents)
	assert.Equal(t, new("invoice_paid"), draft.GoalEvent)
	assert.Equal(t, []string{recoveryExit, "plan_downgraded", "email.clicked"}, live.ExitEvents)
	assert.Equal(t, new(recoveryGoal), live.GoalEvent)

	cleared, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{GoalEvent: patch.Clear[string]()})
	require.NoError(t, err)
	_, draft = automations.LiveAndDraft(cleared)
	assert.Nil(t, draft.GoalEvent)
}
