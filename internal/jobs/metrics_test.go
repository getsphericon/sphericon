package jobs_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/mailjobs"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// Queue depth counts the jobs ready to run per queue, and the oldest available age is how long
// the oldest of them has waited. A job scheduled for later is not ready and a running job is no
// longer waiting, so neither counts: their queues, like every empty queue, report 0.
func TestQueueMetricsReportDepthAndOldestAge(t *testing.T) {
	e := newRiverEnvWithRealQueue(t)
	metricstest.StartMetrics(t)
	unregister, err := jobs.RegisterQueueMetrics(e.pool)
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Report("unregister the gauge", unregister()) })

	ctx := t.Context()
	waiting := time.Now().Add(-time.Minute)
	insertJob(t, e.pool, river.QueueDefault, rivertype.JobStateAvailable, waiting)
	require.NoError(t, e.jobs.Enqueue(ctx, mailjobs.Welcome("b@example.com", "B")))
	later := time.Now().Add(time.Hour)
	_, err = e.queue.Insert(ctx, broadcasts.SendArgs{BroadcastID: fixtures.BroadcastDraftID, ScheduledAt: &later},
		&river.InsertOpts{Queue: broadcasts.Queue, ScheduledAt: later})
	require.NoError(t, err)
	insertJob(t, e.pool, jobs.QueueWebhooks, rivertype.JobStateRunning, waiting)

	scrape := metricstest.ScrapeMetrics(t)
	queue := map[string]string{"queue": river.QueueDefault}
	assert.InDelta(t, 2, scrape.Value(t, "river_queue_depth", queue), 0)
	assert.InDelta(t, time.Minute.Seconds(), scrape.Value(t, "river_queue_oldest_available_age_seconds", queue), 30,
		"the age is the oldest available job's wait")
	for _, idle := range []string{broadcasts.Queue, jobs.QueueWebhooks} {
		q := map[string]string{"queue": idle}
		assert.Zero(t, scrape.Value(t, "river_queue_depth", q), "queue %s has nothing ready", idle)
		assert.Zero(t, scrape.Value(t, "river_queue_oldest_available_age_seconds", q), "queue %s has nothing waiting", idle)
	}
}

// insertJob puts a job in state into queue, due since scheduled. river's insert API makes a job
// with a time of its own scheduled, however long ago, so this goes through the driver's full
// insert, which takes a state.
func insertJob(t *testing.T, pool *sql.DB, queue string, state rivertype.JobState, scheduled time.Time) {
	t.Helper()
	_, err := riverdatabasesql.New(pool).GetExecutor().JobInsertFull(t.Context(), &riverdriver.JobInsertFullParams{
		EncodedArgs: []byte(`{}`),
		Kind:        "waiting_for_the_gauge",
		MaxAttempts: river.MaxAttemptsDefault,
		Priority:    river.PriorityDefault,
		Queue:       queue,
		ScheduledAt: &scheduled,
		State:       state,
	})
	require.NoError(t, err)
}
