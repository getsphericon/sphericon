package testhelper

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// The jobs a test queues wait in its transaction until PerformEnqueuedJobs (perform.go) works
// them; a test that needs a job waiting in the queue puts one there and reads it back through
// these helpers. They use
// river's own insert-only client over the test's connection (the same txdb connection the
// ent client rides), so nothing here touches the river table directly.

// kindsOf is the kind of each args.
func kindsOf(args []river.JobArgs) []string {
	kinds := make([]string, len(args))
	for i, a := range args {
		kinds[i] = a.Kind()
	}
	return kinds
}

// maxQueuedJobs bounds a JobsOf read; a test queues a handful.
// A test helper read bound; not in the contract.
const maxQueuedJobs = 1000

func (env *TestEnv) riverClient(tb testing.TB) *river.Client[*sql.Tx] {
	tb.Helper()
	return RiverClientOn(tb, env.SQLDB)
}

// RiverClientOn is river's insert-only client over sqlDB with no middleware, for a test that
// queues or reads jobs on a database of its own (RealDB): it queues even a job a production
// client would refuse, such as one of a Workspace that names none.
func RiverClientOn(tb testing.TB, sqlDB *sql.DB) *river.Client[*sql.Tx] {
	tb.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(sqlDB), &river.Config{})
	require.NoError(tb, err)
	return rc
}

// EnqueueJob puts one available job of args into the queue.
func (env *TestEnv) EnqueueJob(t *testing.T, args river.JobArgs) {
	t.Helper()
	_, err := env.riverClient(t).Insert(t.Context(), args, nil)
	require.NoError(t, err)
}

// EnqueueJobs puts one available job of each args into the queue, in one insert.
func (env *TestEnv) EnqueueJobs(t *testing.T, args ...river.JobArgs) {
	t.Helper()
	params := make([]river.InsertManyParams, len(args))
	for i, a := range args {
		params[i] = river.InsertManyParams{Args: a}
	}
	_, err := env.riverClient(t).InsertMany(t.Context(), params)
	require.NoError(t, err)
}

// EnqueueScheduledJob puts one job of args into the queue scheduled for at, as a job queued for
// later (a scheduled Broadcast, an Enrollment's wait) waits.
func (env *TestEnv) EnqueueScheduledJob(t *testing.T, args river.JobArgs, at time.Time) {
	t.Helper()
	_, err := env.riverClient(t).Insert(t.Context(), args, &river.InsertOpts{ScheduledAt: at})
	require.NoError(t, err)
}

// EnqueueRunningJob puts one job of args into the queue already in the running state, as
// a worker holds it.
func (env *TestEnv) EnqueueRunningJob(t *testing.T, args river.JobArgs) {
	t.Helper()
	env.enqueueIn(t, args, rivertype.JobStateRunning)
}

// EnqueueDiscardedJob puts one job of args into the queue already discarded, as river leaves
// a job that ran out of attempts.
func (env *TestEnv) EnqueueDiscardedJob(t *testing.T, args river.JobArgs) {
	t.Helper()
	env.enqueueIn(t, args, rivertype.JobStateDiscarded)
}

// enqueueIn puts one job of args into its queue (the args' own InsertOpts, else the default) in
// state. river's insert API only creates waiting jobs, so this goes through the driver's full
// insert, which takes a state.
func (env *TestEnv) enqueueIn(t *testing.T, args river.JobArgs, state rivertype.JobState) {
	t.Helper()
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	queue := river.QueueDefault
	if withOpts, ok := args.(river.JobArgsWithInsertOpts); ok && withOpts.InsertOpts().Queue != "" {
		queue = withOpts.InsertOpts().Queue
	}
	now := time.Now()
	var finalized *time.Time
	if state == rivertype.JobStateDiscarded {
		finalized = &now
	}
	_, err = riverdatabasesql.New(env.SQLDB).GetExecutor().JobInsertFull(t.Context(), &riverdriver.JobInsertFullParams{
		Attempt:     1,
		AttemptedAt: &now,
		AttemptedBy: []string{"test-worker"},
		EncodedArgs: encoded,
		FinalizedAt: finalized,
		Kind:        args.Kind(),
		MaxAttempts: river.MaxAttemptsDefault,
		Priority:    river.PriorityDefault,
		Queue:       queue,
		State:       state,
	})
	require.NoError(t, err)
}

// JobsOf returns the args of the queued jobs of kind, oldest first.
func (env *TestEnv) JobsOf(t *testing.T, kind string) []map[string]any {
	t.Helper()
	return listArgs(t, env.riverClient(t), river.NewJobListParams().Kinds(kind))
}

// listArgs is the args of the jobs params selects, oldest first. It fails the test when the read
// reaches its bound, so a test never asserts on part of the queue.
func listArgs(t *testing.T, rc *river.Client[*sql.Tx], params *river.JobListParams) []map[string]any {
	t.Helper()
	res, err := rc.JobList(t.Context(), params.OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(maxQueuedJobs))
	require.NoError(t, err)
	require.Less(t, len(res.Jobs), maxQueuedJobs, "the test queued more jobs than a read returns")
	out := make([]map[string]any, 0, len(res.Jobs))
	for _, job := range res.Jobs {
		var args map[string]any
		require.NoError(t, json.Unmarshal(job.EncodedArgs, &args))
		out = append(out, args)
	}
	return out
}

// ErrBroadcastWaits is what TryDeliverQueuedBroadcasts returns when a Broadcast's job waits
// instead of ending: a Hold (the Broadcast records its reason), a Deferral or a live claim.
var ErrBroadcastWaits = errors.New("testhelper: a Broadcast job waits")

// maxBroadcastJobRuns bounds a delivery: every recipient's job runs at most its attempts.
const maxBroadcastJobRuns = maxQueuedJobs * broadcasts.RecipientAttempts

// DeliverQueuedBroadcasts works the send jobs of the Broadcasts waiting in the queue through the
// Instance's Outbound module (DeliverQueuedBroadcastsThrough) and fails the test unless each ended.
// Nothing works the queue under txdb, so a test that sends a Broadcast and then needs it
// delivered calls this.
func (env *TestEnv) DeliverQueuedBroadcasts(t *testing.T) {
	t.Helper()
	require.NoError(t, env.TryDeliverQueuedBroadcasts(t))
}

// TryDeliverQueuedBroadcasts is DeliverQueuedBroadcasts for a test that expects the send to wait
// (a Hold): it returns ErrBroadcastWaits, or a plan job's error, instead of failing the test.
func (env *TestEnv) TryDeliverQueuedBroadcasts(t *testing.T) error {
	t.Helper()
	return env.DeliverQueuedBroadcastsThrough(t, env.sender)
}

// DeliverQueuedBroadcastsThrough works the queued plan jobs of the Broadcasts and the recipients'
// jobs they queue through mod, by their real workers, oldest first and whatever their schedule
// says, so a paced recipient is sent at once. A failed send is retried until its attempts run out
// and the recipient is given up, as river would. It stops at the first job that waits
// (ErrBroadcastWaits) and at a plan job's error.
func (env *TestEnv) DeliverQueuedBroadcastsThrough(t *testing.T, mod *outboundmessages.Module) error {
	t.Helper()
	plan := broadcastWorker(t, env, jobs.NewSendBroadcastWorker(env.DB, mod))
	send := broadcastWorker(t, env, jobs.NewSendRecipientWorker(env.DB, mod))
	workers := map[string]performFunc{broadcasts.SendArgs{}.Kind(): plan, jobs.SendRecipientArgs{}.Kind(): send}
	params := river.NewJobListParams().Kinds(broadcasts.SendArgs{}.Kind(), jobs.SendRecipientArgs{}.Kind()).
		States(rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStateRetryable).
		OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(1)
	rc := env.riverClient(t)
	for range maxBroadcastJobRuns {
		res, err := rc.JobList(t.Context(), params)
		require.NoError(t, err)
		if len(res.Jobs) == 0 {
			return nil
		}
		job := res.Jobs[0]
		worked, workErr := env.performOne(t, workers[job.Kind], job)
		if worked != nil && worked.EventKind == river.EventKindJobSnoozed {
			return ErrBroadcastWaits
		}
		if workErr != nil && job.Kind == (broadcasts.SendArgs{}).Kind() {
			return workErr
		}
	}
	require.FailNow(t, "the Broadcast jobs did not run out")
	return nil
}

// broadcastWorker is the performFunc of worker, run with the worker client's configuration.
func broadcastWorker[T river.JobArgs](t *testing.T, env *TestEnv, worker river.Worker[T]) performFunc {
	t.Helper()
	return rivertest.NewWorker(t, riverdatabasesql.New(env.SQLDB), jobs.WorkerConfig(env.DB), worker).WorkJob
}

// SendBroadcastThrough sends Broadcast id as a person's Send does (broadcasts.Module.Send, which
// claims it and queues its plan job) and delivers it through mod (DeliverQueuedBroadcastsThrough).
// A Broadcast already sending, which no Send claims again, gets the plan job its claim queued.
func (env *TestEnv) SendBroadcastThrough(t *testing.T, mod *outboundmessages.Module, id int64) error {
	t.Helper()
	ctx := t.Context()
	b := env.DB.Broadcast.GetX(ctx, id)
	env.claimBroadcast(t, b)
	return env.DeliverQueuedBroadcastsThrough(t, mod)
}

// claimBroadcast queues the plan job of b: through the Broadcasts module's Send, or, for a
// Broadcast already sending, directly.
func (env *TestEnv) claimBroadcast(t *testing.T, b *ent.Broadcast) {
	t.Helper()
	if b.Status == broadcast.StatusSending {
		env.EnqueueJob(t, broadcasts.SendArgs{WorkspaceID: b.WorkspaceID, BroadcastID: b.ID})
		return
	}
	_, err := broadcasts.New(env.Bus, nil, env.sender).Send(t.Context(), env.DB.Scoped(b.WorkspaceID), b.ID, b.Revision)
	require.NoError(t, err)
}

// CancelPendingFor runs an owner's CancelPending for a Contact of the Initech Workspace the
// way Erasure does, inside a purging transaction.
func (env *TestEnv) CancelPendingFor(t *testing.T, contactID int64, cancel func(context.Context, *ent.Scoped, int64, events.QueuePurger) error) error {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.InitechID)
	return env.Bus.WithinScopedPurgeTx(ctx, s, func(ts *ent.Scoped, pub events.PurgingPublisher) error {
		return cancel(ctx, ts, contactID, pub)
	})
}

// RunNextStep takes the oldest waiting step job of an Enrollment from the queue, executes its
// step through mod as the worker would and deletes the job, the way a finished job leaves the
// queue. It reports whether there was a job. The job is run whatever its schedule says: a step
// that is early for its Enrollment's resume time queues itself again, so a test that lets
// waits pass gives the module a clock past them.
func (env *TestEnv) RunNextStep(t *testing.T, mod *automations.StepExecution) bool {
	t.Helper()
	rc := env.riverClient(t)
	res, err := rc.JobList(t.Context(), river.NewJobListParams().
		Kinds(automations.RunStepArgs{}.Kind()).
		States(rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStateRetryable).
		OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(1))
	require.NoError(t, err)
	if len(res.Jobs) == 0 {
		return false
	}
	row := res.Jobs[0]
	var args automations.RunStepArgs
	require.NoError(t, json.Unmarshal(row.EncodedArgs, &args))
	require.NoError(t, jobs.NewRunStepWorker(env.DB, mod).Work(t.Context(), &river.Job[automations.RunStepArgs]{JobRow: row, Args: args}))
	_, err = rc.JobDelete(t.Context(), row.ID)
	require.NoError(t, err)
	return true
}

// JobRowsOf returns the queued jobs of kind as river holds them, oldest first.
func (env *TestEnv) JobRowsOf(t *testing.T, kind string) []*rivertype.JobRow {
	t.Helper()
	res, err := env.riverClient(t).JobList(t.Context(), river.NewJobListParams().Kinds(kind).OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(maxQueuedJobs))
	require.NoError(t, err)
	return res.Jobs
}

// JobSchedules returns when the queued jobs of kind are due, oldest first.
func (env *TestEnv) JobSchedules(t *testing.T, kind string) []time.Time {
	t.Helper()
	rows := env.JobRowsOf(t, kind)
	out := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ScheduledAt)
	}
	return out
}

// RunQueuedSteps runs the waiting step jobs through mod, oldest first, until none is left: the
// steps an Enrollment's own jobs queue are run too, so an Automation is driven to its end
// (or to the HTTP step job, which this does not run).
func (env *TestEnv) RunQueuedSteps(t *testing.T, mod *automations.StepExecution) {
	t.Helper()
	for range maxQueuedJobs {
		if !env.RunNextStep(t, mod) {
			return
		}
	}
	require.FailNow(t, "the queued steps did not run out")
}
