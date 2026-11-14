package jobs_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// noFence is the Fencer of the ordinary tests, whose database is one transaction on one
// connection: no second writer exists there, and a lock held on that connection would starve
// the effect's own queries. The tests below use a database with real connections.
func noFence(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
	return effect(ctx)
}

// settled reports whether ch is closed, without waiting.
func settled(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// blockingSender holds every send in flight until released, and tells when it is.
type blockingSender struct {
	fakeSender

	started chan struct{}
	gate    chan struct{}
	release func()
	done    chan struct{}
}

func newBlockingSender() *blockingSender {
	gate := make(chan struct{})
	return &blockingSender{
		started: make(chan struct{}), gate: gate, release: sync.OnceFunc(func() { close(gate) }), done: make(chan struct{}),
	}
}

func (b *blockingSender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	close(b.started)
	<-b.gate
	receipt, err := b.fakeSender.Send(ctx, msg)
	close(b.done)
	return receipt, err
}

// realEnv is the step executor over a database of the test's own with real connections, so an
// exit and a step are two writers that really contend for the Enrollment's row.
type realEnv struct {
	client *ent.Client
	scoped *ent.Scoped
	fence  automations.Fencer
	url    string
}

func newRealEnv(t *testing.T) *realEnv {
	t.Helper()
	url := testhelper.RealDBURL(t)
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	client := entclient.Core.New(sqlDB)
	return &realEnv{client: client, scoped: client.Scoped(fixtures.AcmeID), fence: automations.FenceOn(client), url: url}
}

// bus is a bus of its own over a pool of its own, with a queue to insert the jobs it enqueues into.
func (e *realEnv) bus(t *testing.T) *events.Bus {
	t.Helper()
	return e.busQueueingThrough(t, func(q events.JobQueue) events.JobQueue { return q })
}

// busQueueingThrough is bus with its queue wrapped by wrap, which sees every job the bus's
// transactions insert.
func (e *realEnv) busQueueingThrough(t *testing.T, wrap func(events.JobQueue) events.JobQueue) *events.Bus {
	t.Helper()
	sqlDB, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "sqlDB") })
	bus := events.New(sqlDB, entclient.Core.NewTx, time.Now)
	queue, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	bus.UseJobs(wrap(queue))
	return bus
}

// stepper is the Step execution over a provider that sends through sender.
func (e *realEnv) stepper(t *testing.T, sender messaging.EmailSender) *automations.StepExecution {
	t.Helper()
	bus := e.bus(t)
	mod := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), resolvingTo(sender), messagetoken.New("test-secret", "http://local", time.Now), time.Now)
	return automations.NewStepExecution(bus, mod, e.fence, nil, nil, time.Now)
}

// enrollments is the Step execution over bus for a race that hands it an Event: it ends
// and makes Enrollments on that bus's own connection, and runs no step.
func (e *realEnv) enrollments(bus *events.Bus) *automations.StepExecution {
	return automations.NewStepExecution(bus, nil, e.fence, nil, nil, time.Now)
}

// enrolledNow enrolls the Contact in the Automation as a Rule whose Event is no longer stored does
// (the Enrollment begins now and no Event began it), and returns the Enrollment.
func (e *realEnv) enrolledNow(t *testing.T, automationID, contactID int64) int64 {
	t.Helper()
	ctx := context.Background()
	in := automations.EnrollOnRuleInput{AutomationID: automationID, ContactID: contactID}
	enrolled, err := e.enrollments(e.bus(t)).EnrollOnRule(ctx, e.scoped, in)
	require.NoError(t, err)
	require.True(t, enrolled)
	run, err := e.client.AutomationRun.Query().
		Where(automationrun.AutomationID(automationID), automationrun.ContactID(contactID)).
		Only(ctx)
	require.NoError(t, err)
	return run.ID
}

// runRowIsLocked reports whether another writer would have to wait for the Enrollment's row:
// the lock an exit's update needs is refused at once (NOWAIT) instead of queued.
func (e *realEnv) runRowIsLocked(t *testing.T, runID int64) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := e.client.Tx(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.AutomationRun.Query().Where(automationrun.ID(runID)).
		ForUpdate(entsql.WithLockAction(entsql.NoWait)).IDs(ctx)
	return err != nil
}

// exitResult is how an exit ended and whether the send had already finished by then.
type exitResult struct {
	err          error
	afterTheSend bool
	ended        bool
}

// An exit that arrives while a step's mail is in flight waits for the mail: the send is
// linearized before the exit, so exactly one mail goes out for the step and none after it.
func TestExitDuringAnEmailStepWaitsForTheSendAndNothingFollows(t *testing.T) {
	e := newRealEnv(t)
	ctx := context.Background()
	bs := newBlockingSender()
	t.Cleanup(bs.release) // a failed assertion must not leave the send blocked
	stepper := e.stepper(t, bs)
	runID := e.enrolledNow(t, fixtures.AutomationWelcomeSeriesID, fixtures.ContactAliceID)

	stepDone := make(chan error, 1)
	go func() {
		stepDone <- stepper.Step(ctx, e.scoped, runID, 0)
	}()
	<-bs.started
	require.True(t, e.runRowIsLocked(t, runID), "an in-flight send holds the Enrollment")

	exited := make(chan exitResult, 1)
	pub := testhelper.PublisherOn(e.bus(t))
	go func() {
		ended, err := automations.Exit(ctx, e.scoped, pub, fixtures.AutomationWelcomeSeriesID, fixtures.ContactAliceID, time.Now())
		exited <- exitResult{err: err, afterTheSend: settled(bs.done), ended: ended}
	}()
	bs.release()

	require.NoError(t, <-stepDone)
	got := <-exited
	require.NoError(t, got.err)
	assert.True(t, got.afterTheSend, "the exit returned only after the send had finished")
	assert.True(t, got.ended)
	assert.Equal(t, automationrun.StatusExited, e.client.AutomationRun.GetX(ctx, runID).Status)
	assert.Len(t, bs.sent, 1, "exactly one mail for the step")

	require.NoError(t, stepper.Step(ctx, e.scoped, runID, 0))
	assert.Len(t, bs.sent, 1, "no later step sends")
}

// The same order holds for an HTTP step: the call is in flight under the Enrollment's lock.
func TestExitDuringAnHTTPStepWaitsForTheCall(t *testing.T) {
	e := newRealEnv(t)
	ctx := context.Background()
	cipher := testhelper.Cipher(t)
	pool, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(pool, "pool") })
	queue := testhelper.RiverClientOn(t, pool)
	workCtx := rivertest.WorkContext(ctx, queue)

	rcv := newHookReceiver(t)
	started, gate := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release) // a failed assertion must not leave the receiver's handler blocked
	rcv.onCall = func() {
		close(started)
		<-gate
	}
	steps := []automationstep.Step{{
		Type: automationstep.HTTP, ID: "01J00000000000000000000001", URL: rcv.srv.URL, Body: `{"ok":true}`,
		Headers: []httpcall.Header{{Name: "Authorization"}},
	}}
	bus := e.bus(t)
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), resolvingTo(&fakeSender{}), messagetoken.New("test-secret", "http://local", time.Now), time.Now)
	mod := automations.New(cipher, bus, testhelper.Renderer(), out, time.Now)
	_, err = mod.Update(ctx, e.scoped, fixtures.AutomationNotifySlackID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(ctx, t, e.scoped, fixtures.AutomationNotifySlackID), Steps: &steps})
	require.NoError(t, err)
	// The edit is a draft until it is published (ADR 0065); the Enrollment begins on the live one.
	_, err = mod.Publish(ctx, e.scoped, fixtures.AutomationNotifySlackID, testhelper.AutomationRevisionIn(ctx, t, e.scoped, fixtures.AutomationNotifySlackID))
	require.NoError(t, err)
	runID := e.enrolledNow(t, fixtures.AutomationNotifySlackID, fixtures.ContactAliceID)
	w := jobs.NewRunHTTPStepWorker(e.client, automations.NewStepExecution(bus, nil, e.fence, cipher, webhook.NewClientAllowing(2*time.Second, rcv.allow), time.Now), lowestDraw)

	workDone := make(chan error, 1)
	go func() {
		workDone <- w.Work(workCtx, &river.Job[automations.RunHTTPStepArgs]{
			JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: automations.RunHTTPStepArgs{}.InsertOpts().MaxAttempts, CreatedAt: time.Now()},
			Args:   automations.RunHTTPStepArgs{RunID: runID},
		})
	}()
	<-started
	require.True(t, e.runRowIsLocked(t, runID), "an in-flight call holds the Enrollment")

	exited := make(chan error, 1)
	pub := testhelper.PublisherOn(e.bus(t))
	go func() {
		_, err := automations.Exit(ctx, e.scoped, pub, fixtures.AutomationNotifySlackID, fixtures.ContactAliceID, time.Now())
		exited <- err
	}()
	release()

	require.NoError(t, <-workDone)
	require.NoError(t, <-exited)
	assert.Equal(t, automationrun.StatusExited, e.client.AutomationRun.GetX(ctx, runID).Status)
	assert.Len(t, rcv.received(), 1, "exactly one call for the step")
}
