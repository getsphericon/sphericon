package testhelper

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/stepping"
)

// Tests work the jobs a request queues at once (enqueue.go), so a test that needs a job to be
// waiting in the queue puts one there and reads it back through these helpers. They use
// river's own insert-only client over the test's connection (the same txdb connection the
// ent client rides), so nothing here touches the river table directly.

// maxQueuedJobs bounds a JobsOf read; a test queues a handful.
// contract-copy: a test helper read bound; not in the contract
const maxQueuedJobs = 1000

func (env *TestEnv) riverClient(t *testing.T) *river.Client[*sql.Tx] {
	t.Helper()
	rc, err := river.NewClient(riverdatabasesql.New(env.SQLDB), &river.Config{})
	require.NoError(t, err)
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

// EnqueueRunningJob puts one job of args into the queue already in the running state, as
// a worker holds it. river's insert API only creates waiting jobs, so this goes through
// the driver's full insert, which takes a state.
func (env *TestEnv) EnqueueRunningJob(t *testing.T, args river.JobArgs) {
	t.Helper()
	encoded, err := json.Marshal(args)
	require.NoError(t, err)
	now := time.Now()
	_, err = riverdatabasesql.New(env.SQLDB).GetExecutor().JobInsertFull(t.Context(), &riverdriver.JobInsertFullParams{
		Attempt:     1,
		AttemptedAt: &now,
		AttemptedBy: []string{"test-worker"},
		EncodedArgs: encoded,
		Kind:        args.Kind(),
		MaxAttempts: river.MaxAttemptsDefault,
		Priority:    river.PriorityDefault,
		Queue:       river.QueueDefault,
		State:       rivertype.JobStateRunning,
	})
	require.NoError(t, err)
}

// JobsOf returns the args of the queued jobs of kind, oldest first.
func (env *TestEnv) JobsOf(t *testing.T, kind string) []map[string]any {
	t.Helper()
	res, err := env.riverClient(t).JobList(t.Context(), river.NewJobListParams().Kinds(kind).OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(maxQueuedJobs))
	require.NoError(t, err)
	out := make([]map[string]any, 0, len(res.Jobs))
	for _, job := range res.Jobs {
		var args map[string]any
		require.NoError(t, json.Unmarshal(job.EncodedArgs, &args))
		out = append(out, args)
	}
	return out
}

// DeliverQueuedBroadcasts runs the send of every Broadcast that has a send job waiting in the
// queue, the way a worker would (synchronously, ignoring the schedule), and returns once each
// has been planned and its recipients sent. Nothing works the queue under txdb, so a test that
// sends a Broadcast and then needs it delivered calls this.
func (env *TestEnv) DeliverQueuedBroadcasts(t *testing.T) {
	t.Helper()
	require.NoError(t, env.TryDeliverQueuedBroadcasts(t))
}

// TryDeliverQueuedBroadcasts is DeliverQueuedBroadcasts for a test that expects the send to be
// refused (a Hold): it returns the first error a send returns instead of failing the test.
func (env *TestEnv) TryDeliverQueuedBroadcasts(t *testing.T) error {
	t.Helper()
	ids := map[int64]bool{}
	for _, args := range env.JobsOf(t, broadcasts.SendArgs{}.Kind()) {
		id, ok := args["broadcast_id"].(float64)
		require.True(t, ok, "a send job names its Broadcast")
		ids[int64(id)] = true // a schedule and a later send queue two jobs for one Broadcast
	}
	for id := range ids {
		if err := jobs.SendBroadcast(t.Context(), env.DB, env.sender, id); err != nil {
			return err
		}
	}
	return nil
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
func (env *TestEnv) RunNextStep(t *testing.T, mod *stepping.Module) bool {
	t.Helper()
	rc := env.riverClient(t)
	res, err := rc.JobList(t.Context(), river.NewJobListParams().
		Kinds(stepping.RunStepArgs{}.Kind()).
		States(rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStateRetryable).
		OrderBy(river.JobListOrderByID, river.SortOrderAsc).First(1))
	require.NoError(t, err)
	if len(res.Jobs) == 0 {
		return false
	}
	row := res.Jobs[0]
	var args stepping.RunStepArgs
	require.NoError(t, json.Unmarshal(row.EncodedArgs, &args))
	require.NoError(t, jobs.NewRunStepWorker(env.DB, mod).Work(t.Context(), &river.Job[stepping.RunStepArgs]{JobRow: row, Args: args}))
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
func (env *TestEnv) RunQueuedSteps(t *testing.T, mod *stepping.Module) {
	t.Helper()
	for range maxQueuedJobs {
		if !env.RunNextStep(t, mod) {
			return
		}
	}
	require.FailNow(t, "the queued steps did not run out")
}
