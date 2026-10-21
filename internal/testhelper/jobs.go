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
)

// Tests run jobs inline, so a test that needs a job to be waiting in the queue puts
// one there and reads it back through these helpers. They use river's own insert-only
// client over the test's connection (the same txdb connection the ent client rides),
// so nothing here touches the river table directly.

// maxQueuedJobs bounds a JobsOf read; a test queues a handful.
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
	var out []map[string]any
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
	ids := map[int64]bool{}
	for _, args := range env.JobsOf(t, broadcasts.SendArgs{}.Kind()) {
		id, ok := args["broadcast_id"].(float64)
		require.True(t, ok, "a send job names its Broadcast")
		ids[int64(id)] = true // a schedule and a later send queue two jobs for one Broadcast
	}
	for id := range ids {
		require.NoError(t, jobs.SendBroadcast(t.Context(), env.DB, env.sender, id))
	}
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
