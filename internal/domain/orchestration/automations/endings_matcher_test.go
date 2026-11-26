package automations_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/ruleaction"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ending is the matchers of an Automation version that end an Enrollment (ADR 0133).
type ending struct {
	exits []eventmatcher.Matcher
	goal  *eventmatcher.Matcher
}

// liveEnding creates an Automation on the Trigger whose one Step waits an hour, so its Enrollment
// stays active, ended by the matchers of e, and publishes it. It returns its id.
func liveEnding(t *testing.T, env *testhelper.TestEnv, trigger eventmatcher.Matcher, e ending) int64 {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Ending on " + trigger.Action, Trigger: automations.EventTrigger{Matcher: trigger}, ExitEvents: e.exits, GoalEvent: e.goal,
		Steps: steps(automationstep.Wait{Seconds: 3600}),
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	return a.ID
}

// collected is a collected Event of the Contact with the action and properties, occurred at.
func collected(contactID int64, action string, at time.Time, properties map[string]any) *events.CollectedEvent {
	return &events.CollectedEvent{WorkspaceID: fixtures.AcmeID, ContactID: contactID, Action: action, Properties: properties, OccurredAt: at}
}

// onlyRunOf is the status of the Contact's one Enrollment in the Automation.
func onlyRunOf(t *testing.T, env *testhelper.TestEnv, automationID, contactID int64) automationrun.Status {
	t.Helper()
	runs := runsOf(t, env, automationID, contactID)
	require.Len(t, runs, 1)
	return runs[0].Status
}

// handLast hands the last stored Event of the action to the Automations again, as a redelivered
// trigger job does.
func handLast(t *testing.T, env *testhelper.TestEnv, contactID int64, action string) {
	t.Helper()
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	last := stored[len(stored)-1]
	in := automations.OnEventInput{ContactID: contactID, Event: automations.Event{Action: last.Action, SourceID: *last.SourceID, OccurredAt: last.OccurredAt}}
	require.NoError(t, testhelper.Module[*automations.StepExecution](env).OnEvent(t.Context(), env.DB.Scoped(fixtures.AcmeID), in))
}

// A Trigger on contact.tagged where tag is trial and a goal on contact.tagged where tag is
// qualified: the triggering Event does not end the Enrollment, tagging qualified reaches the goal.
func TestATagGoalOnTheTriggersActionReachesTheGoalOnItsTagOnly(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	for _, name := range []string{"trial", "qualified"} {
		_, err := tags.New().EnsureIn(t.Context(), s, name)
		require.NoError(t, err)
	}
	id := liveEnding(t, env, eventmatcher.Matcher{Action: systemevent.ContactTagged, Filter: propertyEq(t, "tag", "trial")},
		ending{goal: &eventmatcher.Matcher{Action: systemevent.ContactTagged, Filter: propertyEq(t, "tag", "qualified")}})
	c := enrollable(t, s, 0)

	handStored(t, env, c.ID, systemevent.ContactTagged, tagged(c.ID, "trial"))
	afterTheTrigger := onlyRunOf(t, env, id, c.ID)
	handStored(t, env, c.ID, systemevent.ContactTagged, tagged(c.ID, "vip"))
	afterAnotherTag := onlyRunOf(t, env, id, c.ID)
	handStored(t, env, c.ID, systemevent.ContactTagged, tagged(c.ID, "qualified"))
	afterQualified := onlyRunOf(t, env, id, c.ID)

	assert.Equal(t, automationrun.StatusActive, afterTheTrigger)
	assert.Equal(t, automationrun.StatusActive, afterAnotherTag)
	assert.Equal(t, automationrun.StatusGoalReached, afterQualified)
}

// A goal on the Trigger's own matcher means "did it again": the Event that began the Enrollment
// never ends it, as it is made or when it is handled again; the next one reaches the goal.
func TestTheTriggeringEventNeverEndsItsEnrollment(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	order := eventmatcher.Matcher{Action: "order.completed"}
	id := liveEnding(t, env, order, ending{goal: &order})
	c := enrollable(t, s, 0)

	handStored(t, env, c.ID, "order.completed", collected(c.ID, "order.completed", time.Now().Add(-time.Minute), nil))
	asMade := onlyRunOf(t, env, id, c.ID)
	handLast(t, env, c.ID, "order.completed")
	handledAgain := onlyRunOf(t, env, id, c.ID)
	handStored(t, env, c.ID, "order.completed", collected(c.ID, "order.completed", time.Now(), nil))
	again := onlyRunOf(t, env, id, c.ID)

	assert.Equal(t, automationrun.StatusActive, asMade)
	assert.Equal(t, automationrun.StatusActive, handledAgain)
	assert.Equal(t, automationrun.StatusGoalReached, again)
}

// closing is an Automation on order.placed whose exits are order.closed where status is cancelled
// and order.closed where channel is phone, and whose goal is order.closed where status is paid.
func closing(t *testing.T, env *testhelper.TestEnv) int64 {
	t.Helper()
	return liveEnding(t, env, eventmatcher.Matcher{Action: "order.placed"}, ending{
		exits: []eventmatcher.Matcher{
			{Action: "order.closed", Filter: propertyEq(t, "status", "cancelled")},
			{Action: "order.closed", Filter: propertyEq(t, "channel", "phone")},
		},
		goal: &eventmatcher.Matcher{Action: "order.closed", Filter: propertyEq(t, "status", "paid")},
	})
}

// An exit and the goal on one action with different filters each end the Enrollment as their own
// outcome; an Event that matches neither ends nothing, and one that matches both reaches the goal.
func TestAnExitAndTheGoalOnOneActionEndByTheirFilters(t *testing.T) {
	cases := []struct {
		name       string
		properties map[string]any
		want       automationrun.Status
	}{
		{"cancelled exits", map[string]any{"status": "cancelled"}, automationrun.StatusExited},
		{"paid reaches the goal", map[string]any{"status": "paid"}, automationrun.StatusGoalReached},
		{"pending ends nothing", map[string]any{"status": "pending"}, automationrun.StatusActive},
		{"paid by phone matches both and reaches the goal", map[string]any{"status": "paid", "channel": "phone"}, automationrun.StatusGoalReached},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			id := closing(t, env)
			c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
			handStored(t, env, c.ID, "order.placed", collected(c.ID, "order.placed", time.Now().Add(-time.Minute), nil))

			handStored(t, env, c.ID, "order.closed", collected(c.ID, "order.closed", time.Now(), tc.properties))

			status := onlyRunOf(t, env, id, c.ID)
			assert.Equal(t, tc.want, status)
		})
	}
}

// An exit or goal Event recorded after the triggering Event occurred, but before the Enrollment was
// made, ends it as it is made, its filters applied: the first matching one says how.
func TestAnEnrollmentIsMadeEndedByTheFirstMatchingEventSinceItsTrigger(t *testing.T) {
	cases := []struct {
		name  string
		since []map[string]any // the order.closed Events recorded since the trigger, in order
		want  automationrun.Status
	}{
		{"none matches", []map[string]any{{"status": "pending"}}, automationrun.StatusActive},
		{"a pending order, then a paid one", []map[string]any{{"status": "pending"}, {"status": "paid"}}, automationrun.StatusGoalReached},
		{"a cancelled order, then a paid one", []map[string]any{{"status": "cancelled"}, {"status": "paid"}}, automationrun.StatusExited},
		{"one matching both", []map[string]any{{"status": "paid", "channel": "phone"}}, automationrun.StatusGoalReached},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			id := closing(t, env)
			c := enrollable(t, env.DB.Scoped(fixtures.AcmeID), 0)
			triggered := time.Now().Add(-time.Hour)
			require.NoError(t, env.Publisher().Publish(t.Context(), collected(c.ID, "order.placed", triggered, nil)))
			for i, properties := range tc.since {
				at := triggered.Add(time.Duration(i+1) * time.Minute)
				require.NoError(t, env.Publisher().Publish(t.Context(), collected(c.ID, "order.closed", at, properties)))
			}

			handLast(t, env, c.ID, "order.placed")

			status := onlyRunOf(t, env, id, c.ID)
			assert.Equal(t, tc.want, status)
		})
	}
}

// firedOn publishes a Rule that enrolls Contacts in the Automation on the action and has it fire on
// the last stored Event of that action for the Contact. It returns the Firing's id and the Event's
// source id.
func firedOn(
	t *testing.T, env *testhelper.TestEnv, automationID, contactID int64, action string,
) (firingID int64, sourceID string) {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*rules.Module](env)
	r, err := m.Create(ctx, s, rules.CreateInput{
		Name: "Enroll on " + action, TriggerEvents: []string{action},
		Actions: []ruleaction.Action{{Type: ruleaction.Enroll, AutomationID: automationID}},
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, r.ID, testhelper.RuleRevisionIn(ctx, t, s, r.ID))
	require.NoError(t, err)
	stored := env.StoredEvents(t, action)
	require.NotEmpty(t, stored)
	sourceID = *stored[len(stored)-1].SourceID
	fired, err := m.Evaluate(ctx, s, rules.Event{ContactID: contactID, Action: action, SourceID: sourceID, RecordedAt: time.Now()})
	require.NoError(t, err)
	require.Len(t, fired, 1)
	return fired[0], sourceID
}

// A Rule's Enroll Action that fired on an Event is begun by that Event as a Trigger's Enrollment is
// by its own: an Automation whose goal is the Event's action is not made ended by it, nor ended
// when the Event is handled again; the next such Event reaches the goal.
func TestTheEventARuleFiredOnNeverEndsTheEnrollmentItMade(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	order := eventmatcher.Matcher{Action: "order.completed"}
	id := liveEnding(t, env, eventmatcher.Matcher{Action: "order.placed"}, ending{goal: &order})
	c := enrollable(t, s, 0)
	handStored(t, env, c.ID, "order.completed", collected(c.ID, "order.completed", time.Now().Add(-time.Minute), nil))
	firingID, sourceID := firedOn(t, env, id, c.ID, "order.completed")

	_, err := testhelper.Module[*automations.StepExecution](env).EnrollOnRule(t.Context(), s,
		automations.EnrollOnRuleInput{AutomationID: id, ContactID: c.ID, FiringID: firingID, SourceID: sourceID})
	require.NoError(t, err)
	asMade := onlyRunOf(t, env, id, c.ID)
	handLast(t, env, c.ID, "order.completed")
	handledAgain := onlyRunOf(t, env, id, c.ID)
	handStored(t, env, c.ID, "order.completed", collected(c.ID, "order.completed", time.Now(), nil))
	again := onlyRunOf(t, env, id, c.ID)

	assert.Equal(t, automationrun.StatusActive, asMade)
	assert.Equal(t, automationrun.StatusActive, handledAgain)
	assert.Equal(t, automationrun.StatusGoalReached, again)
}
