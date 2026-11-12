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
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
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

// handEvent hands an Event of the Contact with the action, occurred at, to the Automations, as its
// trigger job does: it ends what the Event ends and enrolls into what it starts.
func handEvent(t *testing.T, env *testhelper.TestEnv, contactID int64, action string, at time.Time) {
	t.Helper()
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: action, OccurredAt: at}}
	require.NoError(t, env.StepExecution.OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), in))
}

// runsOf is the Contact's Enrollments in the Automation.
func runsOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) []*ent.AutomationRun {
	t.Helper()
	runs, err := env.DB.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		All(context.Background())
	require.NoError(t, err)
	return runs
}

// triggerRecovery hands the Contact's payment_failed, occurred at, to the Automations and returns
// the one Enrollment in Payment recovery it made.
func triggerRecovery(t *testing.T, env *testhelper.TestEnv, contactID int64, at time.Time) int64 {
	t.Helper()
	handEvent(t, env, contactID, "payment_failed", at)
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, contactID)
	require.Len(t, runs, 1)
	return runs[0].ID
}

// recoveryRun is the active Enrollment in Payment recovery the fixtures hold, begun a day ago, and
// its Contact.
func recoveryRun(t *testing.T, env *testhelper.TestEnv) (runID, contactID int64) {
	t.Helper()
	run, err := env.DB.AutomationRun.Get(context.Background(), fixtures.AutomationRunRecoveryActiveID)
	require.NoError(t, err)
	return run.ID, run.ContactID
}

func TestTheGoalEventEndsTheEnrollmentAsGoalReached(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, contactID := recoveryRun(t, env)

	handEvent(t, env, contactID, recoveryGoal, time.Now())
	ended, err := s.AutomationRun().Get(ctx, runID)
	require.NoError(t, err)
	require.NoError(t, env.StepExecution.Step(ctx, s, runID, ended.CurrentStep))

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusGoalReached, status)
	queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())
	assert.Empty(t, queued, "no later step runs")
}

func TestAnExitEventEndsTheEnrollmentAsExited(t *testing.T) {
	env := testhelper.Setup(t)
	runID, contactID := recoveryRun(t, env)

	handEvent(t, env, contactID, recoveryExit, time.Now())
	exited := statusOf(t, env, runID)
	handEvent(t, env, contactID, recoveryGoal, time.Now())
	afterTheGoal := statusOf(t, env, runID)

	assert.Equal(t, automationrun.StatusExited, exited)
	assert.Equal(t, automationrun.StatusExited, afterTheGoal, "an ended Enrollment stays as it ended")
}

// An Enrollment begins when its triggering Event occurred: history from before then, loaded or
// sent late, does not end it.
func TestAnEventThatOccurredBeforeTheEnrollmentBeganEndsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	c := enrollable(t, s, 0)
	triggered := time.Now().Add(-time.Hour)
	runID := triggerRecovery(t, env, c.ID, triggered)

	handEvent(t, env, c.ID, recoveryGoal, triggered.Add(-time.Second))
	handEvent(t, env, c.ID, recoveryExit, triggered.Add(-time.Second))

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
	runID := triggerRecovery(t, env, c.ID, triggered)
	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	require.Equal(t, automationrun.StatusActive, run.Status)
	paidBeforeWritten := paid.Before(run.CreatedAt)
	require.True(t, paidBeforeWritten, "the payment occurred before the run was written")

	handEvent(t, env, c.ID, recoveryGoal, paid)

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
	runID := triggerRecovery(t, env, c.ID, triggered)

	handEvent(t, env, c.ID, recoveryGoal, paid)

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
	closed, err := env.DB.Event.Query().
		Where(event.ContactID(fixtures.ContactPaidBeforeEnrolledID), event.Action(recoveryExit)).
		Only(ctx)
	require.NoError(t, err)

	runID := triggerRecovery(t, env, fixtures.ContactPaidBeforeEnrolledID, time.Now().Add(-6*24*time.Hour))

	run, err := env.DB.AutomationRun.Get(ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, automationrun.StatusExited, run.Status)
	assert.Equal(t, &closed.OccurredAt, run.EndedAt)
}

// A step ends an Enrollment at the moment the Step execution's clock gives.
func TestAStepEndsAnEnrollmentAtTheStepExecutionsClock(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	at := time.Date(2026, 3, 12, 9, 30, 0, 0, time.UTC)
	steps := automations.NewStepExecution(env.Bus, env.OutboundModule(), noFence, env.Cipher, nil, func() time.Time { return at })

	// The Enrollment walks its two tag steps and completes.
	require.NoError(t, steps.Step(ctx, s, fixtures.AutomationRunTagOnEngagementAliceID, 0))
	env.RunQueuedSteps(t, steps)

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
		name       string
		triggered  time.Duration
		wantStatus automationrun.Status
		wantQueued int      // the jobs of a first step: one when a step is to run
		wantEvents []string // the System events the Enrollment records (#897)
	}{
		{"the first since the trigger is an exit", 6 * day, automationrun.StatusExited, 0,
			[]string{systemevent.AutomationEnrolled, systemevent.AutomationExited}},
		{"the first since the trigger is the goal", 4 * day, automationrun.StatusGoalReached, 0,
			[]string{systemevent.AutomationEnrolled, systemevent.AutomationGoalReached}},
		{"only a machine click and loaded history since", 2 * day, automationrun.StatusActive, 1,
			[]string{systemevent.AutomationEnrolled}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)

			runID := triggerRecovery(t, env, fixtures.ContactPaidBeforeEnrolledID, time.Now().Add(-tc.triggered))
			status := statusOf(t, env, runID)
			handEvent(t, env, fixtures.ContactPaidBeforeEnrolledID, "payment_failed", time.Now())
			runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaidBeforeEnrolledID)
			queued := env.JobsOf(t, automations.RunStepArgs{}.Kind())

			assert.Len(t, queued, tc.wantQueued, "a step is to run")
			assert.Equal(t, tc.wantStatus, status)
			assert.Len(t, runs, 1, "enrolled once ever: no second Enrollment is made")
			recorded := lo.Map(env.QueuedEnvelopes(t,
				systemevent.AutomationEnrolled, systemevent.AutomationExited, systemevent.AutomationGoalReached),
				func(e events.Envelope, _ int) string { return e.Name })
			assert.Equal(t, tc.wantEvents, recorded, "entered, and left at once when made ended")
		})
	}
}

// Matching is by Contact and by the actions the version lists, nothing else.
func TestAnUnlistedEventOrAnotherContactsEventEndsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	runID, contactID := recoveryRun(t, env)
	other := int64(fixtures.ContactBobID)

	handEvent(t, env, contactID, "payment_failed", time.Now())
	handEvent(t, env, other, recoveryGoal, time.Now())

	status := statusOf(t, env, runID)
	assert.Equal(t, automationrun.StatusActive, status)
}

// An Enrollment reads its exit and goal events from the version it stands on: a later Publish
// that changes them, or a Deactivate, does not change what ends it.
func TestAnEnrollmentEndsByTheEventsOfTheVersionItStandsOn(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, contactID := recoveryRun(t, env)

	_, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID),
		ExitEvents: &[]string{"card_removed"}, GoalEvent: patch.Clear[string](),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, fixtures.AutomationPaymentRecoveryID, testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID))
	require.NoError(t, err)
	_, err = m.Deactivate(ctx, s, fixtures.AutomationPaymentRecoveryID)
	require.NoError(t, err)

	handEvent(t, env, contactID, "card_removed", time.Now())
	afterTheNewExit := statusOf(t, env, runID)
	handEvent(t, env, contactID, recoveryGoal, time.Now())
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

	enrolled, err := env.StepExecution.EnrollOnRule(ctx, s, automations.EnrollOnRuleInput{
		AutomationID: fixtures.AutomationPaymentRecoveryID, ContactID: fixtures.ContactPaidBeforeEnrolledID, SourceID: "evt-not-stored",
	})
	require.NoError(t, err)
	latest := time.Now()
	runs := runsOf(t, env, fixtures.AutomationPaymentRecoveryID, fixtures.ContactPaidBeforeEnrolledID)
	require.Len(t, runs, 1)
	run := runs[0]

	assert.True(t, enrolled)
	assert.Equal(t, automationrun.StatusActive, run.Status)
	assert.WithinRange(t, run.BeganAt, earliest, latest)
}

func TestCreateKeepsExitAndGoalEventsInTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	goal := " subscription_expanded "

	created, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Upsell", TriggerEvent: "plan_limit_reached",
		ExitEvents: []string{"account_closed", " account_closed", systemevent.EmailUnsubscribed}, GoalEvent: &goal,
	})
	require.NoError(t, err)

	_, draft := automations.LiveAndDraft(created)
	require.NotNil(t, draft)
	assert.Equal(t, []string{"account_closed", systemevent.EmailUnsubscribed}, draft.ExitEvents, "trimmed, a repeat kept once")
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
		{"an infrastructure exit", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{systemevent.EmailSent}}, automations.ErrInvalidExitEvent},
		{"a goal equal to the trigger", automations.CreateInput{TriggerEvent: "payment_failed", GoalEvent: new("payment_failed")}, automations.ErrInvalidGoalEvent},
		{"an infrastructure goal", automations.CreateInput{TriggerEvent: "payment_failed", GoalEvent: new(systemevent.EmailDelivered)}, automations.ErrInvalidGoalEvent},
		{"a goal that is also an exit", automations.CreateInput{TriggerEvent: "payment_failed", ExitEvents: []string{"paid"}, GoalEvent: new("paid")}, automations.ErrInvalidGoalEvent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
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
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID),
		TriggerEvent: new(recoveryExit),
	})

	require.ErrorIs(t, err, automations.ErrInvalidExitEvent)
}

// Exit and goal events are the definition (ADR 0065): editing them writes the draft and leaves the
// live version as it was.
func TestEditingExitAndGoalEventsWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule(), time.Now)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	updated, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID),
		ExitEvents: &[]string{}, GoalEvent: patch.Set("invoice_paid"),
	})
	require.NoError(t, err)

	live, draft := automations.LiveAndDraft(updated)
	require.NotNil(t, draft)
	assert.Empty(t, draft.ExitEvents)
	assert.Equal(t, new("invoice_paid"), draft.GoalEvent)
	assert.Equal(t, []string{recoveryExit, "plan_downgraded", systemevent.EmailClicked}, live.ExitEvents)
	assert.Equal(t, new(recoveryGoal), live.GoalEvent)

	cleared, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID), GoalEvent: patch.Clear[string]()})
	require.NoError(t, err)
	_, draft = automations.LiveAndDraft(cleared)
	assert.Nil(t, draft.GoalEvent)
}
