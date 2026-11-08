package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// holdEventLog stops every read of the Event log until the returned release is called. An Event
// handed to the Automations reads its stored row (its properties, #752) after it has picked the
// Automations it starts and before it enrolls the Contact into them, so the hold keeps it in that
// window.
func holdEventLog(t *testing.T, e *realEnv) (release func()) {
	t.Helper()
	db, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(db, "event log hold") })
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	// raw sql: a table lock that also stops plain reads; ent has no statement for it.
	_, err = tx.ExecContext(t.Context(), "LOCK TABLE events IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, err)
	release = sync.OnceFunc(func() { _ = tx.Rollback() })
	t.Cleanup(release) // a failed assertion must not leave the Event handler blocked
	return release
}

// raceThePick hands Alice's hold_demo to the Automations, runs meanwhile once they have picked Hold
// demo and before they enroll her, then lets them go on. It returns Alice's Enrollments in Hold
// demo.
func raceThePick(t *testing.T, meanwhile func(ctx context.Context, mod *automations.Module, s *ent.Scoped) error) []int64 {
	t.Helper()
	e := newRealEnv(t)
	ctx := context.Background()
	bus := e.bus(t)
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), resolvingTo(&fakeSender{}), messagetoken.New("test-secret", "http://local", time.Now), time.Now)
	mod := automations.New(testhelper.Cipher(t), bus, testhelper.Renderer(), out, time.Now)
	steps := e.enrollments(e.bus(t))
	probe, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(probe, "probe") })
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
	require.NoError(t, <-handled)

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
		_, updated := mod.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{TriggerEvent: new("hold_demo_moved")})
		_, published := mod.Publish(ctx, s, fixtures.AutomationHoldDemoID)
		return errors.Join(updated, published)
	})

	assert.Empty(t, runs)
}
