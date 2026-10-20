package jobs_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/tracking"
	"github.com/getsphericon/sphericon/internal/webhook"
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
	fence  jobs.Fencer
	url    string
}

func newRealEnv(t *testing.T) *realEnv {
	t.Helper()
	url := testhelper.RealDBURL(t)
	sqlDB, err := sql.Open("pgx", url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	client := db.NewEntClient(sqlDB)
	return &realEnv{client: client, scoped: client.Scoped(fixtures.AcmeID), fence: jobs.FenceOn(client), url: url}
}

func (e *realEnv) mod(t *testing.T, sender messaging.EmailSender) *outbound.Module {
	t.Helper()
	sqlDB, err := sql.Open("pgx", e.url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return outbound.New(events.New(sqlDB), resolvingTo(sender), tracking.New("test-secret", "http://local"))
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
	mod := e.mod(t, bs)
	runID, enrolled, err := automations.Enroll(ctx, e.scoped, fixtures.AutomationWelcomeSeriesID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)

	stepDone := make(chan error, 1)
	go func() {
		_, err := jobs.RunStep(ctx, e.client, e.fence, mod, runID)
		stepDone <- err
	}()
	<-bs.started
	require.True(t, e.runRowIsLocked(t, runID), "an in-flight send holds the Enrollment")

	exited := make(chan exitResult, 1)
	go func() {
		ended, err := automations.Exit(ctx, e.scoped, fixtures.AutomationWelcomeSeriesID, fixtures.ContactAliceID)
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

	res, err := jobs.RunStep(ctx, e.client, e.fence, mod, runID)
	require.NoError(t, err)
	assert.True(t, res.Done)
	assert.Len(t, bs.sent, 1, "no later step sends")
}

// The same order holds for an HTTP step: the call is in flight under the Enrollment's lock.
func TestExitDuringAnHTTPStepWaitsForTheCall(t *testing.T) {
	e := newRealEnv(t)
	ctx := context.Background()
	cfg, err := config.Load("test")
	require.NoError(t, err)
	cipher, err := secrets.NewCipher(cfg.EncryptionKey)
	require.NoError(t, err)
	pool, err := pgxpool.New(ctx, e.url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	queue, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)
	workCtx := rivertest.WorkContext(ctx, queue)

	rcv := newHookReceiver(t)
	started, gate := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release) // a failed assertion must not leave the receiver's handler blocked
	rcv.onCall = func() {
		close(started)
		<-gate
	}
	steps := []automations.Step{{
		Type: automations.StepHTTP, ID: "01J00000000000000000000001", URL: rcv.srv.URL, Body: `{"ok":true}`,
		Headers: []automations.Header{{Name: "Authorization"}},
	}}
	_, err = automations.New(cipher).Update(ctx, e.scoped, fixtures.AutomationNotifySlackID, automations.UpdateInput{Steps: &steps})
	require.NoError(t, err)
	runID, enrolled, err := automations.Enroll(ctx, e.scoped, fixtures.AutomationNotifySlackID, fixtures.ContactAliceID)
	require.NoError(t, err)
	require.True(t, enrolled)
	w := jobs.NewRunHTTPStepWorker(e.client, e.fence, automations.New(cipher), webhook.NewClientAllowing(2*time.Second, rcv.allow), time.Now)

	workDone := make(chan error, 1)
	go func() {
		workDone <- w.Work(workCtx, &river.Job[jobs.RunHTTPStepArgs]{
			JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: jobs.RunHTTPStepArgs{}.InsertOpts().MaxAttempts, CreatedAt: time.Now()},
			Args:   jobs.RunHTTPStepArgs{RunID: runID},
		})
	}()
	<-started
	require.True(t, e.runRowIsLocked(t, runID), "an in-flight call holds the Enrollment")

	exited := make(chan error, 1)
	go func() { exited <- automations.ExitRun(ctx, e.scoped, runID) }()
	release()

	require.NoError(t, <-workDone)
	require.NoError(t, <-exited)
	assert.Equal(t, automationrun.StatusExited, e.client.AutomationRun.GetX(ctx, runID).Status)
	assert.Len(t, rcv.received(), 1, "exactly one call for the step")
}

// An exit committed before a step takes the lock sends nothing: the lock is taken on the
// active row, so the step finds the Enrollment gone from `active`.
func TestFenceOnAnExitedEnrollmentRunsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	runID := exitedRun(t, env, fixtures.AutomationHoldDemoID, fixtures.ContactAliceID)
	ran := make(chan struct{})

	err := jobs.FenceOn(env.DB)(context.Background(), env.DB.Scoped(fixtures.AcmeID), runID, func(context.Context) error {
		close(ran)
		return nil
	})

	require.True(t, automations.Stale(err))
	assert.False(t, settled(ran))
}
