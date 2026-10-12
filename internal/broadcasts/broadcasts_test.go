package broadcasts_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type enqueued struct {
	id int64
	at *time.Time
}

// recorder is the job-queue seam: it records what was enqueued, or fails.
type recorder struct {
	calls []enqueued
	err   error
}

func (r *recorder) EnqueueBroadcast(_ context.Context, id int64, at *time.Time) error {
	if r.err != nil {
		return r.err
	}
	r.calls = append(r.calls, enqueued{id, at})
	return nil
}

func TestSendMovesDraftToSendingAndEnqueuesImmediately(t *testing.T) {
	env := testhelper.Setup(t)
	q := &recorder{}
	m := broadcasts.New(q)

	b, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusSending, b.Status)
	assert.Equal(t, []enqueued{{fixtures.BroadcastDraftID, nil}}, q.calls)
}

func TestSendClearsTheScheduleOfAScheduledBroadcast(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})

	b, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastScheduledID)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusSending, b.Status)
	assert.Nil(t, b.ScheduledAt)
}

func TestSendRevertsToDraftWhenEnqueueFails(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("queue down")
	m := broadcasts.New(&recorder{err: boom})

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, broadcast.StatusDraft, env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastDraftID).Status)
}

func TestScheduleSetsTimeAndEnqueuesForThen(t *testing.T) {
	env := testhelper.Setup(t)
	q := &recorder{}
	m := broadcasts.New(q)
	when := time.Now().Add(24 * time.Hour).Truncate(time.Second)

	b, err := m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, when)
	require.NoError(t, err)

	assert.Equal(t, broadcast.StatusScheduled, b.Status)
	require.NotNil(t, b.ScheduledAt)
	assert.True(t, when.Equal(*b.ScheduledAt))
	require.Len(t, q.calls, 1)
	assert.EqualValues(t, fixtures.BroadcastDraftID, q.calls[0].id)
	require.NotNil(t, q.calls[0].at)
	assert.True(t, when.Equal(*q.calls[0].at))
}

func TestScheduleRevertsToDraftAndClearsTimeWhenEnqueueFails(t *testing.T) {
	env := testhelper.Setup(t)
	boom := errors.New("queue down")
	m := broadcasts.New(&recorder{err: boom})

	_, err := m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastDraftID, time.Now().Add(time.Hour))

	require.ErrorIs(t, err, boom)
	got := env.DB.Broadcast.GetX(context.Background(), fixtures.BroadcastDraftID)
	assert.Equal(t, broadcast.StatusDraft, got.Status)
	assert.Nil(t, got.ScheduledAt)
}

func TestTransitionsOutOfSendingAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	q := &recorder{}
	m := broadcasts.New(q)

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSendingID)
	assert.ErrorIs(t, err, broadcasts.ErrNotSendable)
	_, err = m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.BroadcastSendingID, time.Now().Add(time.Hour))
	assert.ErrorIs(t, err, broadcasts.ErrNotSendable)
	assert.Empty(t, q.calls)
}

func TestUnscheduleReturnsAScheduledBroadcastToDraft(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})

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
	m := broadcasts.New(&recorder{})

	for _, id := range []int64{fixtures.BroadcastDraftID, fixtures.BroadcastSendingID} {
		_, err := m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), id)
		assert.ErrorIs(t, err, broadcasts.ErrNotScheduled, "broadcast %d", id)
	}
	_, err := m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), 999999)
	assert.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Unschedule(context.Background(), env.DB.Scoped(fixtures.AcmeID+1000), fixtures.BroadcastScheduledID)
	assert.ErrorIs(t, err, broadcasts.ErrNotFound, "another workspace's broadcast")
}

func TestUnknownOrForeignBroadcastIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	m := broadcasts.New(&recorder{})

	_, err := m.Send(context.Background(), env.DB.Scoped(fixtures.AcmeID+1000), fixtures.BroadcastDraftID)
	assert.ErrorIs(t, err, broadcasts.ErrNotFound)
	_, err = m.Schedule(context.Background(), env.DB.Scoped(fixtures.AcmeID), 999999, time.Now().Add(time.Hour))
	assert.ErrorIs(t, err, broadcasts.ErrNotFound)
}
