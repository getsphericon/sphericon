package testhelper

import (
	"context"
	"database/sql"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// A test works the jobs it queued the way ActiveJob's perform_enqueued_jobs does: the code under
// test queues through the real river client, the jobs wait in the test's transaction, and
// PerformEnqueuedJobs works them with their real workers through river's own test executor
// (rivertest.Worker.WorkJob): args decoded, the application's middleware and the worker's timeout
// applied, the job completed (or set to retry) in the queue.

// maxPerformRounds bounds PerformEnqueuedJobs: each round works the jobs the previous one queued.
const maxPerformRounds = 20

// performFunc works one queued job in tx, as rivertest.Worker.WorkJob does.
type performFunc func(ctx context.Context, tb testing.TB, tx *sql.Tx, job *rivertype.JobRow) (*rivertest.WorkResult, error)

// performers are the workers PerformEnqueuedJobs works the jobs of, by kind, built over the test's
// connection with the test's senders. river keeps the kind of each worker it is given to itself,
// so the list is written out here; a kind missing from it fails PerformEnqueuedJobs by name.
type performers map[string]performFunc

// add registers worker under the kind of its args, worked with the worker client's configuration
// (jobs.WorkerConfig over client).
func add[T river.JobArgs](tb testing.TB, p performers, sqlDB *sql.DB, client *ent.Client, worker river.Worker[T]) {
	tb.Helper()
	var args T
	p[args.Kind()] = rivertest.NewWorker(tb, riverdatabasesql.New(sqlDB), jobs.WorkerConfig(client), worker).WorkJob
}

// PerformEnqueuedJobs works the jobs waiting in the test's queue, oldest first, each by its real
// worker in a transaction on the test's connection, until none is left, so a job a job queues is
// worked too. With only, it works the jobs of those kinds and leaves the others waiting. A worker
// that fails, or a waiting job with no worker in performers, fails the test.
func (env *TestEnv) PerformEnqueuedJobs(t *testing.T, only ...river.JobArgs) {
	t.Helper()
	env.performEnqueuedJobs(t, true, only...)
}

// mailKinds are the jobs of the Notices (ADR 0105), which SystemMail runs before a read: the
// Notices' event consumer, which queues the sends, and the sends.
var mailKinds = []river.JobArgs{jobs.NoticeEventArgs{}, jobs.SendNoticeArgs{}}

// performMail works the queued Notices (mailKinds) for SystemMail, leaving one whose worker fails
// to retry.
func (env *TestEnv) performMail() {
	env.tb.Helper()
	env.performEnqueuedJobs(env.tb, false, mailKinds...)
}

// performEnqueuedJobs is PerformEnqueuedJobs; without mustSucceed a job whose worker fails is left
// to retry, as river leaves it, and the test goes on.
func (env *TestEnv) performEnqueuedJobs(tb testing.TB, mustSucceed bool, only ...river.JobArgs) {
	tb.Helper()
	env.performEnqueuedJobsWhere(tb, mustSucceed, nil, only...)
}

// PerformEnqueuedJobsNewestFirst exercises consumers without a FIFO assumption, using the same
// River executor and middleware as PerformEnqueuedJobs.
func (env *TestEnv) PerformEnqueuedJobsNewestFirst(t *testing.T, only ...river.JobArgs) {
	t.Helper()
	env.performEnqueuedJobsInOrder(t, true, nil, river.SortOrderDesc, only...)
}

// performEnqueuedJobsWhere is performEnqueuedJobs for the waiting jobs that match, or all of them
// when match is nil.
func (env *TestEnv) performEnqueuedJobsWhere(tb testing.TB, mustSucceed bool, match func(*rivertype.JobRow) bool, only ...river.JobArgs) {
	tb.Helper()
	env.performEnqueuedJobsInOrder(tb, mustSucceed, match, river.SortOrderAsc, only...)
}

func (env *TestEnv) performEnqueuedJobsInOrder(tb testing.TB, mustSucceed bool, match func(*rivertype.JobRow) bool, order river.SortOrder, only ...river.JobArgs) {
	tb.Helper()
	params := river.NewJobListParams().States(rivertype.JobStateAvailable).
		OrderBy(river.JobListOrderByID, order).First(maxQueuedJobs)
	if len(only) > 0 {
		params = params.Kinds(kindsOf(only)...)
	}
	rc := env.riverClient(tb)
	workers := env.performers(tb)
	// A second connection while the jobs run, which txdb maps onto the same transaction: river's
	// test executor holds a nested transaction on one while the worker's queries take the other
	// (river discussion #1214); with one, the worker would wait forever. Only for this: two
	// goroutines' nested transactions on one transaction would interleave.
	env.SQLDB.SetMaxOpenConns(2)
	defer env.SQLDB.SetMaxOpenConns(1)
	for range maxPerformRounds {
		res, err := rc.JobList(tb.Context(), params)
		require.NoError(tb, err)
		worked := 0
		for _, job := range res.Jobs {
			if match != nil && !match(job) {
				continue
			}
			worked++
			perform, ok := workers[job.Kind]
			require.Truef(tb, ok, "testhelper: no worker for the job kind %q in performers", job.Kind)
			env.perform(tb.Context(), tb, perform, job, mustSucceed)
		}
		if worked == 0 {
			return
		}
	}
	tb.Fatalf("testhelper: the jobs still queued jobs after %d rounds", maxPerformRounds)
}

// perform works job in a transaction of its own and keeps what it did, the job's new state
// included, as river does when a job ends.
func (env *TestEnv) perform(ctx context.Context, tb testing.TB, perform performFunc, job *rivertype.JobRow, mustSucceed bool) {
	tb.Helper()
	_, workErr := env.performIn(ctx, tb, perform, job)
	if mustSucceed {
		require.NoError(tb, workErr, job.Kind)
	}
}

// performOne is perform for one job outside PerformEnqueuedJobs: it opens the second connection
// the work needs and returns what the job did and the worker's error.
func (env *TestEnv) performOne(tb testing.TB, perform performFunc, job *rivertype.JobRow) (*rivertest.WorkResult, error) {
	tb.Helper()
	env.SQLDB.SetMaxOpenConns(2)
	defer env.SQLDB.SetMaxOpenConns(1)
	return env.performIn(tb.Context(), tb, perform, job)
}

// performIn works job in a transaction of its own and commits what it did.
func (env *TestEnv) performIn(ctx context.Context, tb testing.TB, perform performFunc, job *rivertype.JobRow) (*rivertest.WorkResult, error) {
	tb.Helper()
	tx, err := env.SQLDB.BeginTx(ctx, nil)
	require.NoError(tb, err)
	res, workErr := perform(ctx, tb, tx, job)
	require.NoError(tb, tx.Commit())
	return res, workErr
}
