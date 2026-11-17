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
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
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

	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusGone))

	require.NoError(t, err)
	stored := r.endpoint()
	assert.Nil(t, stored.DisabledAt)
	assert.Nil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Empty(t, r.disabled())
}

func TestAnOldSuccessCannotClearFailuresAfterAnOwnerReset(t *testing.T) {
	r := newRecording(t)
	r.record(status(http.StatusServiceUnavailable))
	inFlight := r.endpoint()
	_, err := r.mod.Update(t.Context(), r.scope(), inFlight.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, r.scope(), inFlight.ID), Enabled: patch.Set(true)})
	require.NoError(t, err)
	r.record(status(http.StatusBadGateway))

	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, nil)

	require.NoError(t, err)
	stored := r.endpoint()
	require.NotNil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Equal(t, 1, *stored.Edges.Health.ConsecutiveFailures)
}

func TestOverlappingFailuresKeepTheFirstFailureOfTheirSeries(t *testing.T) {
	r := newRecording(t)
	inFlight := r.endpoint()
	began := r.now
	err := r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusServiceUnavailable))
	require.NoError(t, err)

	r.now = began.Add(90 * time.Minute)
	err = r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, status(http.StatusBadGateway))

	require.NoError(t, err)
	stored := r.endpoint()
	require.NotNil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Equal(t, 2, *stored.Edges.Health.ConsecutiveFailures)
	require.NotNil(t, stored.Edges.Health.FailingSince)
	assert.True(t, began.Equal(*stored.Edges.Health.FailingSince))
}

func TestASuccessClearsFailuresRecordedWhileTheRequestWasInFlight(t *testing.T) {
	r := newRecording(t)
	inFlight := r.endpoint()
	r.record(status(http.StatusServiceUnavailable))

	err := r.mod.RecordDelivery(t.Context(), r.scope(), inFlight, nil)

	require.NoError(t, err)
	stored := r.endpoint()
	assert.Nil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Nil(t, stored.Edges.Health.FailingSince)
}

// refusingEventQueue refuses every job the bus queues with fault.
type refusingEventQueue struct {
	events.JobQueue // only the inserts are called

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

	err := r.mod.RecordDelivery(t.Context(), r.scope(), r.endpoint(), status(http.StatusGone))

	require.ErrorIs(t, err, fault)
	stored := r.endpoint()
	assert.Nil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Nil(t, stored.Edges.Health.FailingSince)
	assert.Nil(t, stored.DisabledAt)
	assert.Empty(t, r.disabled())
}

// Pause only the first mutation in each of two transactions, after both loaded their snapshot.
// A third mutation is the losing transaction's retry, which must be free to proceed.
func pauseFirstHealthWrites(ready chan<- struct{}, resume <-chan struct{}) ent.Hook {
	var entered atomic.Int32
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			_, healthWrite := mutation.(*ent.WebhookEndpointHealthMutation)
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
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.AcmeID)
	bus := env.Bus
	mod := testhelper.Module[*webhookendpoints.Module](env)
	inFlight, err := scope.WebhookEndpoint().Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).WithHealth().Only(t.Context())
	require.NoError(t, err)
	ready, resume := make(chan struct{}, 2), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	bus.Use(pauseFirstHealthWrites(ready, resume))
	results := make(chan error, 2)
	for range 2 {
		go func() {
			results <- mod.RecordDelivery(t.Context(), scope, inFlight, status(http.StatusServiceUnavailable))
		}()
	}
	testhelper.Receive(t, ready, "first result loaded its position")
	testhelper.Receive(t, ready, "second result loaded the same position")
	release()

	first := testhelper.Receive(t, results, "first result recorded")
	second := testhelper.Receive(t, results, "second result recorded")

	require.NoError(t, first)
	require.NoError(t, second)
	stored, err := scope.WebhookEndpoint().Query().Where(webhookendpoint.ID(inFlight.ID)).WithHealth().Only(t.Context())
	require.NoError(t, err)
	require.NotNil(t, stored.Edges.Health.ConsecutiveFailures)
	assert.Equal(t, 2, *stored.Edges.Health.ConsecutiveFailures)
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
	assert.Equal(t, before.Edges.Health.ConsecutiveFailures, stored.Edges.Health.ConsecutiveFailures)
	err = r.mod.RecordDelivery(t.Context(), r.scope(), before, status(http.StatusBadGateway))
	require.NoError(t, err)
	continued := r.endpoint()
	require.NotNil(t, continued.Edges.Health.ConsecutiveFailures)
	assert.Equal(t, 2, *continued.Edges.Health.ConsecutiveFailures)
}

func TestResetGenerationStaysOutOfTheConfigurationAudit(t *testing.T) {
	r := newRecording(t)
	r.record(status(http.StatusGone))
	before := r.endpoint()
	require.NotNil(t, before.DisabledAt, "the endpoint turned itself off")
	sys := r.env.Bus.Act(r.scope(), events.Actor{Kind: events.ActorSystem})

	_, err := r.mod.Update(t.Context(), sys, before.ID, webhookendpoints.UpdateInput{Revision: testhelper.WebhookRevisionIn(t.Context(), t, sys, before.ID), Enabled: patch.Set(true)})

	require.NoError(t, err)
	entries := testhelper.Queued[*events.AuditEntry](t, r.env, events.NameAuditEntry)
	require.NotEmpty(t, entries)
	entry := entries[len(entries)-1]
	assert.Contains(t, entry.Diff, "disabled_at")
	assert.NotContains(t, entry.Diff, "health_generation")
	assert.NotContains(t, entry.Diff, "consecutive_failures", "the failure run lives beside the endpoint, unaudited (ADR 0168)")
}

// A delivery result that meets the owner's disable of the endpoint between its read and its write
// takes no place in the run: the run is written only while the endpoint is as the delivery read it.
func TestADeliveryThatMeetsAnOwnerDisableLeavesTheRun(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.AcmeID)
	mod := testhelper.Module[*webhookendpoints.Module](env)
	inFlight, err := scope.WebhookEndpoint().Query().Where(webhookendpoint.ID(fixtures.WebhookCodebasicsID)).WithHealth().Only(t.Context())
	require.NoError(t, err)
	ready, resume := make(chan struct{}, 2), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	t.Cleanup(release)
	env.Bus.Use(pauseFirstHealthWrites(ready, resume))
	result := make(chan error, 1)
	go func() {
		result <- mod.RecordDelivery(t.Context(), scope, inFlight, status(http.StatusServiceUnavailable))
	}()
	testhelper.Receive(t, ready, "the delivery loaded its position")

	_, err = mod.Update(t.Context(), scope, inFlight.ID, webhookendpoints.UpdateInput{
		Revision: testhelper.WebhookRevisionIn(t.Context(), t, scope, inFlight.ID), Enabled: patch.Set(false),
	})
	require.NoError(t, err)
	release()

	require.NoError(t, testhelper.Receive(t, result, "the delivery recorded"))
	stored, err := scope.WebhookEndpoint().Query().Where(webhookendpoint.ID(inFlight.ID)).WithHealth().Only(t.Context())
	require.NoError(t, err)
	assert.NotNil(t, stored.DisabledAt)
	assert.Nil(t, stored.Edges.Health.ConsecutiveFailures, "the run of a disabled endpoint takes no result")
}
