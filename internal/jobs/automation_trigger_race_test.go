package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// holdEventLog stops every read of the Event log until the returned release is called. An Event
// handed to the Automations reads its stored row (its properties, #752) after it has picked the
// Automations it starts and before it enrolls the Contact into them, so the hold keeps it in that
// window.
func holdEventLog(t *testing.T, e *realEnv) (release func()) {
	t.Helper()
	// A table lock that also stops plain reads; the cleanup releases it if an assertion fails, so
	// the Event handler is never left blocked.
	return testhelper.HoldInTransaction(t, e.env.SQLDB, "LOCK TABLE events IN ACCESS EXCLUSIVE MODE")
}

// raceThePick hands Alice's hold_demo to the Automations, runs meanwhile once they have picked Hold
// demo and before they enroll her, then lets them go on. It returns Alice's Enrollments in Hold
// demo.
func raceThePick(t *testing.T, meanwhile func(ctx context.Context, mod *automations.Module, s *ent.Scoped) error) []int64 {
	t.Helper()
	e := newRealEnv(t)
	ctx := context.Background()
	mod := testhelper.Module[*automations.Module](e.env)
	steps := e.enrollments()
	probe := e.env.SQLDB
	release := holdEventLog(t, e)

	handled, done := make(chan error, 1), make(chan struct{})
	go func() {
		handled <- steps.OnEvent(ctx, e.scoped, automations.OnEventInput{
			ContactID: fixtures.ContactAliceID,
			Event:     automations.Event{Action: "hold_demo", SourceID: "evt-hold-demo-race", OccurredAt: time.Now()},
		})
		close(done)
	}()
	require.Eventually(t, func() bool {
		waiting, err := testhelper.LockWaiters(ctx, probe)
		return err == nil && waiting > 0
	}, 10*time.Second, 5*time.Millisecond, "the Event's handler waits, Hold demo picked, before it enrolls")
	stillWaiting := !settled(done)
	require.True(t, stillWaiting, "the Event's handler is held in the window")
	require.NoError(t, meanwhile(ctx, mod, e.scoped))
	release()
	require.NoError(t, testhelper.Receive(t, handled, "the trigger is handled"))

	runs, err := e.client.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationHoldDemoID), automationrun.ContactID(fixtures.ContactAliceID)).
		IDs(ctx)
	require.NoError(t, err)
	return runs
}

// The Automations an Event starts are picked before the Contact is enrolled, so a Deactivate can
// land in between: under the Contact's lock the Enrollment reads the Automation again and, no
// longer active, it enrolls nobody.
func TestADeactivateBetweenPickingAndEnrollingEnrollsNobody(t *testing.T) {
	runs := raceThePick(t, func(ctx context.Context, mod *automations.Module, s *ent.Scoped) error {
		_, err := mod.Deactivate(ctx, s, fixtures.AutomationHoldDemoID)
		return err
	})

	assert.Empty(t, runs)
}

// A Publish that moves the Trigger can land there too: under the lock the Enrollment reads the
// live version again and, started by another action now, it enrolls nobody.
func TestAPublishThatMovesTheTriggerBetweenPickingAndEnrollingEnrollsNobody(t *testing.T) {
	runs := raceThePick(t, func(ctx context.Context, mod *automations.Module, s *ent.Scoped) error {
		_, updated := mod.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationHoldDemoID), Trigger: &automations.Trigger{Event: eventmatcher.Matcher{Action: "hold_demo_moved"}}})
		_, published := mod.Publish(ctx, s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationHoldDemoID))
		return errors.Join(updated, published)
	})

	assert.Empty(t, runs)
}

// A Publish that gives the Trigger a filter the Event misses can land there too: under the lock the
// Enrollment runs the live version's filter again and enrolls nobody.
func TestAPublishThatFiltersTheTriggerBetweenPickingAndEnrollingEnrollsNobody(t *testing.T) {
	leaf, err := json.Marshal(map[string]any{
		"kind": "property", "key": "plan", "test": map[string]any{"kind": "text", "operator": "eq", "value": "pro"},
	})
	require.NoError(t, err)
	filter := &conditiontree.EventFilter{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{leaf}}

	runs := raceThePick(t, func(ctx context.Context, mod *automations.Module, s *ent.Scoped) error {
		_, updated := mod.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationHoldDemoID), Trigger: &automations.Trigger{Event: eventmatcher.Matcher{Action: "hold_demo", Filter: filter}}})
		_, published := mod.Publish(ctx, s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(ctx, t, s, fixtures.AutomationHoldDemoID))
		return errors.Join(updated, published)
	})

	assert.Empty(t, runs)
}
