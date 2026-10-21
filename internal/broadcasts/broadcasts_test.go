package broadcasts_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// sendKind is the kind of the Broadcast send job.
var sendKind = broadcasts.SendArgs{}.Kind()

// failingQueue is a job queue that cannot take the job: it fails the insert, and first runs
// onInsert (a request that dies together with its queue cancels its context there).
type failingQueue struct {
	err      error
	onInsert func()
}

func (q failingQueue) InsertTx(context.Context, *sql.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if q.onInsert != nil {
		q.onInsert()
	}
	return nil, q.err
}

func TestSendMovesDraftToSendingAndQueuesTheJobImmediately(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	b, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusSending, b.Status)
	jobs := env.JobsOf(t, sendKind)
	require.Len(t, jobs, 1)
	assert.EqualValues(t, fixtures.BroadcastDraftID, jobs[0]["broadcast_id"])
	assert.NotContains(t, jobs[0], "scheduled_at", "an immediate send names no time")
}

func TestSendClearsTheScheduleOfAScheduledBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	b, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusSending, b.Status)
	assert.Nil(t, b.ScheduledAt)
}

func TestSendLeavesTheBroadcastADraftWhenTheJobCannotBeQueued(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("queue down")
	env.Bus.UseJobs(failingQueue{err: boom})

	_, err := broadcasts.New(env.Bus).Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastDraftID).Status)
}

// The claim and the job are one transaction, so a request that dies with its queue cannot
// strand the Broadcast in sending: nothing was committed to roll back by hand.
func TestSendLeavesTheBroadcastADraftWhenTheQueueAndTheRequestFailTogether(t *testing.T) {
	env := testhelper.Setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	env.Bus.UseJobs(failingQueue{err: errors.New("queue is down"), onInsert: cancel})

	_, err := broadcasts.New(env.Bus).Send(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)

	require.Error(t, err)
	got := env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status, "a Broadcast whose job was never queued must not stay claimed")
}

func TestScheduleSetsTimeAndQueuesTheJobForThen(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)
	when := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	b, err := m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, when)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusScheduled, b.Status)
	require.NotNil(t, b.ScheduledAt)
	assert.True(t, when.Equal(*b.ScheduledAt))
	jobs := env.JobsOf(t, sendKind)
	require.Len(t, jobs, 1)
	assert.EqualValues(t, fixtures.BroadcastDraftID, jobs[0]["broadcast_id"])
	queuedFor, err := time.Parse(time.RFC3339, jobs[0]["scheduled_at"].(string))
	require.NoError(t, err)
	assert.True(t, when.Equal(queuedFor))
}

func TestScheduleLeavesTheBroadcastADraftWithNoTimeWhenTheJobCannotBeQueued(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("queue down")
	env.Bus.UseJobs(failingQueue{err: boom})

	_, err := broadcasts.New(env.Bus).Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, time.Now().Add(time.Hour))

	require.ErrorIs(t, err, boom)
	got := env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status)
	assert.Nil(t, got.ScheduledAt)
}

func TestTransitionsOutOfSendingAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSendingID)
	require.ErrorIs(t, err, broadcasts.ErrNotSendable)
	_, err = m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSendingID, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, broadcasts.ErrNotSendable)
	assert.Empty(t, env.JobsOf(t, sendKind), "a refused claim queues nothing")
}

func TestUnscheduleReturnsAScheduledBroadcastToDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	b, err := m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusDraft, b.Status)
	assert.Nil(t, b.ScheduledAt)
	got := env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastScheduledID)
	assert.Equal(t, broadcast.StatusDraft, got.Status)
	assert.Nil(t, got.ScheduledAt)
}

func TestUnscheduleRefusesWhatIsNotScheduled(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	for _, id := range []int64{fixtures.BroadcastDraftID, fixtures.BroadcastSendingID} {
		_, err := m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), id)
		require.ErrorIs(t, err, broadcasts.ErrNotScheduled, "broadcast %d", id)
	}
	_, err := m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), 999999)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID+1000), fixtures.BroadcastScheduledID)
	assert.ErrorIs(t, err, broadcasts.ErrNotFound, "another workspace's broadcast")
}

func TestUnknownOrForeignBroadcastIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(env.Bus)

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID+1000), fixtures.BroadcastDraftID)
	require.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), 999999, time.Now().Add(time.Hour))
	assert.ErrorIs(t, err, broadcasts.ErrNotFound)
}
