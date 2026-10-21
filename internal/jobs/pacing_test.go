package jobs_test

import (
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
)

// recipientJobTimes returns the scheduled time of every queued recipient job, in
// scheduled order.
func (e *riverEnv) recipientJobTimes(t *testing.T) []time.Time {
	t.Helper()
	res, err := e.client.River().JobList(e.workCtx(),
		river.NewJobListParams().Kinds("send_broadcast_recipient").
			OrderBy(river.JobListOrderByScheduledAt, river.SortOrderAsc).First(1000))
	require.NoError(t, err)
	times := make([]time.Time, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		times = append(times, j.ScheduledAt)
	}
	return times
}

func (e *riverEnv) planBroadcast(t *testing.T, id int64) {
	t.Helper()
	e.DB.Broadcast.UpdateOneID(id).SetStatus(broadcast.StatusSending).ExecX(e.workCtx())
	w := jobs.NewSendBroadcastWorker(e.DB, newMod(e.TestEnv, resolvingTo(e.sender)))
	require.NoError(t, w.Work(e.workCtx(), job(broadcasts.SendArgs{BroadcastID: id})))
}

// The river adapter inserts each recipient job at the time the module computed for it.
func TestRiverAdapterSchedulesRecipientJobsAtTheModulesTimes(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()
	e.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeDefaultID).SetMaxPerSecond(2).ExecX(ctx)

	e.planBroadcast(t, fixtures.BroadcastDraftID)

	times := e.recipientJobTimes(t)
	require.Greater(t, len(times), 2)
	for i := 1; i < len(times); i++ {
		assert.InDelta(t, 500*time.Millisecond, times[i].Sub(times[i-1]), float64(5*time.Millisecond), "one over the rate")
	}
}
