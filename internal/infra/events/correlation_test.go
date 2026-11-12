package events_test

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/logging"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An event published in a request starts a chain named by the request id; one published while
// handling another shares that event's chain and names it as its cause.
func TestAnEventCarriesItsChain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := logging.WithRequestID(t.Context(), "req-chain")

	publishIn(ctx, t, env, "first@example.com")
	first := lastCreated(t, env)
	publishIn(events.Handling(t.Context(), first), t, env, "second@example.com")
	second := lastCreated(t, env)
	publishIn(t.Context(), t, env, "alone@example.com")
	alone := lastCreated(t, env)

	assert.Equal(t, "req-chain", first.CorrelationID)
	assert.Empty(t, first.CausationID, "a chain starts at the request")
	assert.Equal(t, "req-chain", second.CorrelationID)
	assert.Equal(t, first.ID, second.CausationID)
	assert.NotEmpty(t, alone.CorrelationID, "outside a request a chain starts at a fresh id")
	assert.NotEqual(t, "req-chain", alone.CorrelationID)
}

// publishIn publishes a ContactCreated in ctx.
func publishIn(ctx context.Context, t *testing.T, env *testhelper.TestEnv, email string) {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: email})
	}))
}

// lastCreated is the envelope of the last ContactCreated the test published, as its jobs carry it.
func lastCreated(t *testing.T, env *testhelper.TestEnv) events.Envelope {
	t.Helper()
	envelopes := env.QueuedEnvelopes(t, systemevent.ContactCreated)
	return envelopes[len(envelopes)-1]
}

type chainedArgs struct{}

func (chainedArgs) Kind() string { return "chained" }

// A job queued while handling an event keeps the event's chain, and an event the job publishes,
// even in a context of its own, is caused by the handled one.
func TestAJobKeepsTheChainItWasQueuedIn(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	bus := testhelper.BusOn(t, sqlDB)
	cfg := &river.Config{Middleware: []rivertype.Middleware{events.JobCorrelation()}}
	inserter, err := river.NewClient(riverdatabasesql.New(sqlDB), cfg)
	require.NoError(t, err)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	parent := events.Envelope{ID: "01JPARENT", CorrelationID: "01JROOT"}
	queued, err := inserter.InsertTx(events.Handling(ctx, parent), tx, chainedArgs{}, nil)
	require.NoError(t, err)
	w := rivertest.NewWorker(t, riverdatabasesql.New(sqlDB), cfg, river.WorkFunc(func(ctx context.Context, _ *river.Job[chainedArgs]) error {
		return bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
			return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "job@example.com"})
		})
	}))

	_, err = w.WorkJob(ctx, t, tx, queued.Job)

	require.NoError(t, err)
	published := testhelper.QueuedEnvelopesOn(t, sqlDB, systemevent.ContactCreated)
	require.NotEmpty(t, published)
	assert.Equal(t, "01JROOT", published[len(published)-1].CorrelationID)
	assert.Equal(t, "01JPARENT", published[len(published)-1].CausationID)
}
