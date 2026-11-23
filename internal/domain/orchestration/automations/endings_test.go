package automations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
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
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(context.Background(), env.DB.Scoped(fixtures.AcmeID), in))
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
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).Step(ctx, s, runID, lo.FromPtr(ended.CurrentStepID)))

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

// An Enrollment that ends on a Step through an exit or its goal leaves that Step when it ends, in
// the same move (ADR 0124): its record keeps no outcome, the Enrollment's end says why.
func TestAnEnrollmentEndedByAnEventLeavesTheStepItStoodOn(t *testing.T) {
	for _, action := range []string{recoveryExit, recoveryGoal} {
		t.Run(action, func(t *testing.T) {
			env := testhelper.Setup(t)
			runID, contactID := recoveryRun(t, env)
			at := time.Now().Add(-time.Minute).Truncate(time.Microsecond)

			handEvent(t, env, contactID, action, at)

			records, err := testhelper.Module[*automations.Module](env).StepRecords(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, runID)
			require.NoError(t, err)
			require.Len(t, records, 1)
			require.NotNil(t, records[0].LeftAt, "the Step it stood on is left")
			assert.WithinDuration(t, at, *records[0].LeftAt, 0, "when the Event that ended it occurred")
			assert.Nil(t, records[0].Outcome, "the Enrollment's end says why")
		})
	}
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
	steps := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), noFence, env.Cipher, nil, func() time.Time { return at })

	// The Enrollment walks its two tag steps and completes.
	require.NoError(t, steps.Step(ctx, s, fixtures.AutomationRunTagOnEngagementAliceID, env.StepIDAt(t, fixtures.AutomationRunTagOnEngagementAliceID, 0)))
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
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	runID, contactID := recoveryRun(t, env)

	_, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID),
		ExitEvents: &[]eventmatcher.Matcher{{Action: "card_removed"}}, GoalEvent: patch.Clear[eventmatcher.Matcher](),
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

	enrolled, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(ctx, s, automations.EnrollOnRuleInput{
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
	m := testhelper.Module[*automations.Module](env)
	paid := propertyEq(t, "status", "paid")

	created, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), automations.CreateInput{
		Name: "Upsell", Trigger: eventmatcher.Matcher{Action: "plan_limit_reached"},
		ExitEvents: []eventmatcher.Matcher{
			{Action: "account_closed"}, {Action: " account_closed"}, {Action: "account_closed", Filter: paid}, {Action: systemevent.EmailUnsubscribed},
		},
		GoalEvent: &eventmatcher.Matcher{Action: " subscription_expanded "},
	})
	require.NoError(t, err)

	_, draft := automations.LiveAndDraft(created)
	require.NotNil(t, draft)
	want, err := json.Marshal([]eventmatcher.Matcher{{Action: "account_closed"}, {Action: "account_closed", Filter: paid}, {Action: systemevent.EmailUnsubscribed}})
	require.NoError(t, err)
	exits, err := json.Marshal(draft.Exits)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(exits), "trimmed, a repeat kept once, the same action with a filter kept")
	assert.Equal(t, &eventmatcher.Matcher{Action: "subscription_expanded"}, draft.Goal)
}

func TestCreateRefusesAnExitOrGoalEventThatCannotEndAnEnrollment(t *testing.T) {
	trigger := eventmatcher.Matcher{Action: "payment_failed"}
	cases := []struct {
		name string
		in   automations.CreateInput
		want error
	}{
		{"an empty exit", automations.CreateInput{Trigger: trigger, ExitEvents: []eventmatcher.Matcher{{Action: " "}}}, automations.ErrInvalidExitEvent},
		{"an infrastructure exit", automations.CreateInput{Trigger: trigger, ExitEvents: []eventmatcher.Matcher{{Action: systemevent.EmailSent}}}, automations.ErrInvalidExitEvent},
		{"an infrastructure goal", automations.CreateInput{Trigger: trigger, GoalEvent: &eventmatcher.Matcher{Action: systemevent.EmailDelivered}}, automations.ErrInvalidGoalEvent},
		{"an exit naming a Tag the Workspace lacks", automations.CreateInput{Trigger: trigger,
			ExitEvents: []eventmatcher.Matcher{{Action: systemevent.ContactTagged, Filter: propertyEq(t, "tag", "no-such-tag")}}}, automations.ErrInvalidExitEvent},
		{"a goal naming a Custom field the Workspace lacks", automations.CreateInput{Trigger: trigger,
			GoalEvent: &eventmatcher.Matcher{Action: systemevent.ContactFieldChanged, Filter: propertyEq(t, "key", "no_such_field")}}, automations.ErrInvalidGoalEvent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			m := testhelper.Module[*automations.Module](env)
			tc.in.Name = "Refused"

			_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), tc.in)

			require.ErrorIs(t, err, tc.want)
		})
	}
}

// An exit or goal event may share the Trigger's action, and the goal an exit's (ADR 0133): the
// Enrollment's own triggering Event never ends it, and an Event matching both reaches the goal.
func TestASaveAcceptsExitAndGoalEventsSharingAnAction(t *testing.T) {
	trigger := eventmatcher.Matcher{Action: "payment_failed"}
	cases := []struct {
		name string
		in   automations.CreateInput
	}{
		{"an exit on the trigger's action", automations.CreateInput{Trigger: trigger, ExitEvents: []eventmatcher.Matcher{trigger}}},
		{"a goal on the trigger's action", automations.CreateInput{Trigger: trigger, GoalEvent: &trigger}},
		{"a goal on an exit's action", automations.CreateInput{Trigger: trigger,
			ExitEvents: []eventmatcher.Matcher{{Action: "paid"}}, GoalEvent: &eventmatcher.Matcher{Action: "paid"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			m := testhelper.Module[*automations.Module](env)
			tc.in.Name = "Accepted"

			_, err := m.Create(context.Background(), env.DB.Scoped(fixtures.AcmeID), tc.in)

			require.NoError(t, err)
		})
	}
}

// The edit is checked against the definition it makes: a new Trigger on a stored exit event's
// action is accepted, though the edit names no exit event.
func TestUpdateAcceptsATriggerOnAStoredExitEventsAction(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, env.DB.Scoped(fixtures.AcmeID), fixtures.AutomationPaymentRecoveryID),
		Trigger: &eventmatcher.Matcher{Action: recoveryExit},
	})

	require.NoError(t, err)
}

// Exit and goal events are the definition (ADR 0065): editing them writes the draft and leaves the
// live version as it was.
func TestEditingExitAndGoalEventsWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*automations.Module](env)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	updated, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID),
		ExitEvents: &[]eventmatcher.Matcher{}, GoalEvent: patch.Set(eventmatcher.Matcher{Action: "invoice_paid"}),
	})
	require.NoError(t, err)

	live, draft := automations.LiveAndDraft(updated)
	require.NotNil(t, draft)
	assert.Empty(t, draft.Exits)
	assert.Equal(t, &eventmatcher.Matcher{Action: "invoice_paid"}, draft.Goal)
	assert.Equal(t, []eventmatcher.Matcher{{Action: recoveryExit}, {Action: "plan_downgraded"}, {Action: systemevent.EmailClicked}}, live.Exits)
	assert.Equal(t, &eventmatcher.Matcher{Action: recoveryGoal}, live.Goal)

	cleared, err := m.Update(ctx, s, fixtures.AutomationPaymentRecoveryID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationPaymentRecoveryID), GoalEvent: patch.Clear[eventmatcher.Matcher]()})
	require.NoError(t, err)
	_, draft = automations.LiveAndDraft(cleared)
	assert.Nil(t, draft.Goal)
}
