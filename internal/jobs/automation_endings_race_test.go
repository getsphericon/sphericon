package jobs_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// holdingQueue holds the transaction that queues a job open, once the work before the queueing
// is done, until it is released: for an Enrollment, once it is written and has looked for the exit
// and goal Events, and before it commits.
type holdingQueue struct {
	events.JobInserter

	reached chan struct{}
	gate    chan struct{}
	arrive  func()
	release func()
}

func newHoldingQueue() *holdingQueue {
	reached, gate := make(chan struct{}), make(chan struct{})
	return &holdingQueue{
		reached: reached, gate: gate,
		arrive: sync.OnceFunc(func() { close(reached) }), release: sync.OnceFunc(func() { close(gate) }),
	}
}

func (q *holdingQueue) InsertTx(ctx context.Context, tx *sql.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	q.arrive()
	<-q.gate
	return q.JobInserter.InsertTx(ctx, tx, args, opts)
}

// stored writes the Event's row, as the Automations consumer does before it queues the Event's job.
func stored(t *testing.T, e *realEnv, ev events.DomainEvent, occurred time.Time) {
	t.Helper()
	data, err := json.Marshal(ev)
	require.NoError(t, err)
	require.NoError(t, events.Persist(t.Context(), e.client, events.Envelope{
		ID: "evt_paid", Name: ev.EventName(), Version: ev.EventVersion(), WorkspaceID: ev.Workspace(), OccurredAt: occurred, Data: data,
	}))
}

// The goal Event is stored and handled while the trigger's Enrollment is written but not yet
// committed: the Enrollment looked for the payment before it was stored, and the payment's
// evaluation cannot see an uncommitted Enrollment. Both take the Contact's lock, so the payment's
// evaluation waits for the Enrollment to commit and then ends it: it reaches its goal and its
// first step sends nothing (#751).
func TestAGoalEventHandledWhileTheEnrollmentCommitsEndsIt(t *testing.T) {
	e := newRealEnv(t)
	ctx := context.Background()
	hold := newHoldingQueue()
	t.Cleanup(hold.release) // a failed assertion must not leave the enrollment blocked
	enrolling := e.busQueueingThrough(t, func(q events.JobInserter) events.JobInserter {
		hold.JobInserter = q
		return hold
	})
	probe, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(probe, "probe") })
	failed := time.Now().Add(-time.Minute)
	paid := failed.Add(30 * time.Second)

	enrolled := make(chan error, 1)
	go func() {
		enrolled <- stepping.OnEvent(ctx, enrolling, e.scoped, fixtures.ContactAliceID, "payment_failed", 0, failed)
	}()
	<-hold.reached
	stored(t, e, &events.CollectedEvent{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Action: "payment_succeeded", OccurredAt: paid,
	}, paid)
	ending := e.bus(t)
	ended, endDone := make(chan error, 1), make(chan struct{})
	go func() {
		ended <- stepping.OnEvent(ctx, ending, e.scoped, fixtures.ContactAliceID, "payment_succeeded", 0, paid)
		close(endDone)
	}()
	require.Eventually(t, func() bool {
		waiting, err := testhelper.LockWaiters(ctx, probe)
		return settled(endDone) || (err == nil && waiting > 0)
	}, 10*time.Second, 5*time.Millisecond, "the payment's evaluation finished or waits for the Enrollment")
	hold.release()
	require.NoError(t, <-enrolled)
	require.NoError(t, <-ended)

	run, err := e.client.AutomationRun.Query().
		Where(automationrun.AutomationID(fixtures.AutomationPaymentRecoveryID), automationrun.ContactID(fixtures.ContactAliceID)).
		Only(ctx)
	require.NoError(t, err)
	fs := &fakeSender{}
	require.NoError(t, e.stepper(t, fs).Step(ctx, e.scoped, run.ID, 0))
	after, err := e.client.AutomationRun.Get(ctx, run.ID)
	require.NoError(t, err)

	assert.Equal(t, automationrun.StatusGoalReached, after.Status)
	assert.Empty(t, fs.sent, "the first step sends no recovery mail to a payer")
}
