package jobs_test

import (
	"context"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const eventsRetention = 400 * 24 * time.Hour

var (
	// Fixture Events 500 days old that expire at the 400-day window.
	expiring = []int64{
		fixtures.EventOldAnalyticalID,
		fixtures.EventOldAnalyticalSentID,
		fixtures.EventOldAnalyticalTransientBounceID,
		fixtures.EventOldDeliveredID,
		fixtures.EventOldDelayedID,
		fixtures.EventOldDroppedID,
	}
	// Fixture Events 500 days old that are evidentiary and must stay.
	evidentiary = []int64{
		fixtures.EventOldConfirmedID,
		fixtures.EventOldComplainedID,
		fixtures.EventOldUnsubscribedID,
		fixtures.EventOldPermanentBounceID,
	}
)

func pruneEvents(t *testing.T, env *testhelper.TestEnv, retention time.Duration) {
	t.Helper()
	w := jobs.NewPruneEventsWorker(env.DB, retention)
	require.NoError(t, w.Work(context.Background(), &river.Job[jobs.PruneEventsArgs]{JobRow: &rivertype.JobRow{}}))
}

func presentEvents(t *testing.T, env *testhelper.TestEnv, ids []int64) int {
	t.Helper()
	return env.DB.Event.Query().Where(event.IDIn(ids...)).CountX(context.Background())
}

func TestPruneEventsDeletesAnalyticalPastWindowOnly(t *testing.T) {
	env := testhelper.Setup(t)

	pruneEvents(t, env, eventsRetention)

	assert.Zero(t, presentEvents(t, env, expiring), "analytical Events past the window are deleted")
	assert.Equal(t, 1, presentEvents(t, env, []int64{fixtures.EventRecentAnalyticalID}), "younger Events are kept")
}

func TestPruneEventsKeepsEvidentiaryRegardlessOfAge(t *testing.T) {
	env := testhelper.Setup(t)

	pruneEvents(t, env, eventsRetention)

	assert.Equal(t, len(evidentiary), presentEvents(t, env, evidentiary))
}

func TestPruneEventsDisabledByZeroRetention(t *testing.T) {
	env := testhelper.Setup(t)
	before := env.DB.Event.Query().CountX(context.Background())

	pruneEvents(t, env, 0)

	assert.Equal(t, before, env.DB.Event.Query().CountX(context.Background()))
}

func TestPruneEventsIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)

	pruneEvents(t, env, eventsRetention)
	after := env.DB.Event.Query().CountX(context.Background())
	pruneEvents(t, env, eventsRetention)

	assert.Equal(t, after, env.DB.Event.Query().CountX(context.Background()))
}

func TestPruneEventsDeletesInBoundedBatches(t *testing.T) {
	env := testhelper.Setup(t)

	// A batch of one forces several rounds; every expired row still goes.
	deleted, err := events.PruneEvents(context.Background(), env.DB, eventsRetention, 1)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(len(expiring)))
	assert.Zero(t, presentEvents(t, env, expiring))
	assert.Equal(t, len(evidentiary), presentEvents(t, env, evidentiary))
}
