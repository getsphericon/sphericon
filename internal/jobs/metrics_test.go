package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper/metricstest"
)

// Queue depth counts the jobs ready to run per queue; a job scheduled for later
// is not ready, so its queue reports no sample.
func TestQueueMetricsReportDepthAndOldestAge(t *testing.T) {
	e := newRiverEnv(t)
	metricstest.StartMetrics(t)
	unregister, err := jobs.RegisterQueueMetrics(e.pool)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unregister() })

	ctx := t.Context()
	require.NoError(t, e.client.EnqueueWelcome(ctx, "a@example.com", "A"))
	require.NoError(t, e.client.EnqueueWelcome(ctx, "b@example.com", "B"))
	later := time.Now().Add(time.Hour)
	_, err = e.client.River().Insert(ctx, broadcasts.SendArgs{BroadcastID: fixtures.BroadcastDraftID, ScheduledAt: &later},
		&river.InsertOpts{Queue: broadcasts.Queue, ScheduledAt: later})
	require.NoError(t, err)

	scrape := metricstest.ScrapeMetrics(t)
	queue := map[string]string{"queue": river.QueueDefault}
	assert.InDelta(t, 2, scrape.Value(t, "river_queue_depth", queue), 0)
	assert.GreaterOrEqual(t, scrape.Value(t, "river_queue_oldest_available_age_seconds", queue), 0.0)
	assert.False(t, scrape.Has("river_queue_depth", map[string]string{"queue": broadcasts.Queue}), "a job scheduled for later is not ready")
}
