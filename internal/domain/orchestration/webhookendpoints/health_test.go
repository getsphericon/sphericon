package webhookendpoints_test

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAResponseFromBeforeAnOwnerResetCannotDisableTheEndpoint(t *testing.T) {
	r := newRecording(t)
	inFlight := r.endpoint()
	_, err := r.mod.Update(t.Context(), r.scope(), inFlight.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, r.scope(), inFlight.ID), Enabled: patch.Set(true)})
	require.NoError(t, err)

	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusGone), r.now)

	require.NoError(t, err)
	stored := r.endpoint()
	assert.Nil(t, stored.DisabledAt)
	assert.Nil(t, stored.ConsecutiveFailures)
	assert.Empty(t, r.disabled())
}

func TestAnOldSuccessCannotClearFailuresAfterAnOwnerReset(t *testing.T) {
	r := newRecording(t)
	r.record(status(http.StatusServiceUnavailable))
	inFlight := r.endpoint()
	_, err := r.mod.Update(t.Context(), r.scope(), inFlight.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, r.scope(), inFlight.ID), Enabled: patch.Set(true)})
	require.NoError(t, err)
	r.record(status(http.StatusBadGateway))

	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, nil, r.now)

	require.NoError(t, err)
	stored := r.endpoint()
	require.NotNil(t, stored.ConsecutiveFailures)
	assert.Equal(t, 1, *stored.ConsecutiveFailures)
}

func TestOverlappingFailuresKeepTheFirstFailureOfTheirSeries(t *testing.T) {
	r := newRecording(t)
	inFlight := r.endpoint()
	began := r.now
	err := r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusServiceUnavailable), began)
	require.NoError(t, err)

	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusBadGateway), began.Add(90*time.Minute))

	require.NoError(t, err)
	stored := r.endpoint()
	require.NotNil(t, stored.ConsecutiveFailures)
	assert.Equal(t, 2, *stored.ConsecutiveFailures)
	require.NotNil(t, stored.FailingSince)
	assert.True(t, began.Equal(*stored.FailingSince))
}

func TestASuccessClearsFailuresRecordedWhileTheRequestWasInFlight(t *testing.T) {
	r := newRecording(t)
	inFlight := r.endpoint()
	r.record(status(http.StatusServiceUnavailable))

	err := r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, nil, r.now)

	require.NoError(t, err)
	stored := r.endpoint()
	assert.Nil(t, stored.ConsecutiveFailures)
	assert.Nil(t, stored.FailingSince)
}

type refusingEventQueue struct {
	testhelper.AcceptingQueue

	fault error
}

func (q refusingEventQueue) InsertTx(context.Context, *sql.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return nil, q.fault
}

func (q refusingEventQueue) InsertManyTx(context.Context, *sql.Tx, []river.InsertManyParams) ([]*rivertype.JobInsertResult, error) {
	return nil, q.fault
}

func TestFailureCountersRollbackWhenTheDisableEventCannotBeRecorded(t *testing.T) {
	r := newRecording(t)
	fault := errors.New("event queue unavailable")
	r.env.Bus.UseJobs(refusingEventQueue{fault: fault})

	err := r.mod.RecordDelivery(t.Context(), r.scope(), r.endpoint(), status(http.StatusGone), r.now)

	require.ErrorIs(t, err, fault)
	stored := r.endpoint()
	assert.Nil(t, stored.ConsecutiveFailures)
	assert.Nil(t, stored.FailingSince)
	assert.Nil(t, stored.DisabledAt)
	assert.Empty(t, r.disabled())
}

// Pause only the first mutation in each of two transactions, after both loaded their snapshot.
// A third mutation is the losing transaction's retry, which must be free to proceed.
func pauseFirstHealthWrites(ready chan<- struct{}, resume <-chan struct{}) ent.Hook {
	var entered atomic.Int32
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			_, healthWrite := mutation.(*ent.WebhookEndpointMutation)
			if healthWrite && entered.Add(1) <= 2 {
				ready <- struct{}{}
				select {
				case <-resume:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func TestConcurrentCurrentGenerationResultsAreBothCounted(t *testing.T) {
	sqlDB := testhelper.RealDB(t)
	sqlDB.SetMaxOpenConns(4)
	client := entclient.Core.New(sqlDB)
	scope := client.Scoped(fixtures.AcmeID)
	bus := events.New(sqlDB, entclient.Core.NewTx, time.Now)
	mod := webhookendpoints.New(bus, testhelper.Cipher(t), nil, nil, time.Now)
	inFlight, err := scope.WebhookEndpoint().Get(t.Context(), fixtures.WebhookCodebasicsID)
	require.NoError(t, err)
	ready, resume := make(chan struct{}, 2), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	bus.Use(pauseFirstHealthWrites(ready, resume))
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- mod.RecordDelivery(t.Context(), scope, inFlight, status(http.StatusServiceUnavailable), time.Now())
		}()
	}
	testhelper.Receive(t, ready, "first result loaded its position")
	testhelper.Receive(t, ready, "second result loaded the same position")
	release()

	first := testhelper.Receive(t, results, "first result recorded")
	second := testhelper.Receive(t, results, "second result recorded")

	require.NoError(t, first)
	require.NoError(t, second)
	stored, err := scope.WebhookEndpoint().Get(t.Context(), inFlight.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.ConsecutiveFailures)
	assert.Equal(t, 2, *stored.ConsecutiveFailures)
	assert.Nil(t, stored.DisabledAt)
}

func refuseEnable(fault error) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if endpoint, ok := mutation.(*ent.WebhookEndpointMutation); ok && endpoint.DisabledAtCleared() {
				return nil, fault
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func TestAnOwnerResetRollsBackItsGenerationWithItsHealthWrite(t *testing.T) {
	r := newRecording(t)
	r.record(status(http.StatusServiceUnavailable))
	before := r.endpoint()
	fault := errors.New("owner update unavailable")
	r.env.Bus.Use(refuseEnable(fault))

	_, err := r.mod.Update(t.Context(), r.scope(), before.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, r.scope(), before.ID), Enabled: patch.Set(true)})

	require.ErrorIs(t, err, fault)
	stored := r.endpoint()
	assert.Equal(t, before.HealthGeneration, stored.HealthGeneration)
	assert.Equal(t, before.ConsecutiveFailures, stored.ConsecutiveFailures)
	err = r.mod.RecordDelivery(t.Context(), r.scope(), before, status(http.StatusBadGateway), r.now)
	require.NoError(t, err)
	continued := r.endpoint()
	require.NotNil(t, continued.ConsecutiveFailures)
	assert.Equal(t, 2, *continued.ConsecutiveFailures)
}

func TestResetGenerationStaysOutOfTheConfigurationAudit(t *testing.T) {
	r := newRecording(t)
	r.record(status(http.StatusServiceUnavailable))
	before := r.endpoint()
	sys := r.env.Bus.Act(r.scope(), events.Actor{Kind: events.ActorSystem})

	_, err := r.mod.Update(t.Context(), sys, before.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, sys, before.ID), Enabled: patch.Set(true)})

	require.NoError(t, err)
	entries := r.env.QueuedEvents(t, events.NameAuditEntry)
	require.Len(t, entries, 1)
	entry := entries[0].(*events.AuditEntry)
	assert.Contains(t, entry.Diff, "consecutive_failures")
	assert.NotContains(t, entry.Diff, "health_generation")
}
