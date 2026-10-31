package events_test

import (
	"context"
	"testing"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An event published in a request starts a chain named by the request id; one published while
// handling another shares that event's chain and names it as its cause.
func TestAnEventCarriesItsChain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := logging.WithRequestID(t.Context(), "req-chain")

	first := publishIn(ctx, t, env, "first@example.com")
	second := publishIn(events.Handling(t.Context(), first), t, env, "second@example.com")
	alone := publishIn(t.Context(), t, env, "alone@example.com")

	assert.Equal(t, "req-chain", first.CorrelationID)
	assert.Empty(t, first.CausationID, "a chain starts at the request")
	assert.Equal(t, "req-chain", second.CorrelationID)
	assert.Equal(t, first.ID, second.CausationID)
	assert.NotEmpty(t, alone.CorrelationID, "outside a request a chain starts at a fresh id")
	assert.NotEqual(t, "req-chain", alone.CorrelationID)
}

// The router hands a handler the context of its event's chain, and links the handler's span to
// the span that published the event.
func TestAHandlerRunsInItsEventsChainAndLinksToThePublisher(t *testing.T) {
	testhelper.IsolateOtel(t)
	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	sqlDB := testhelper.RealDB(t)
	bus := events.New(sqlDB)
	router, err := events.NewRouter()
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(router, "router") })
	sub, err := events.NewSubscriber(sqlDB, "chain", testhelper.PrivateOutbox())
	require.NoError(t, err)
	handled := make(chan events.Envelope, 2)
	router.AddConsumerHandler("chain", events.TopicDomainEvents, sub, func(msg *message.Message) error {
		env, err := events.EnvelopeOf(msg)
		if err != nil {
			return err
		}
		handled <- env
		if env.CausationID != "" {
			return nil // the follow-up: the chain ends here
		}
		return bus.WithinTx(msg.Context(), func(_ *ent.Client, pub events.Publisher) error {
			return pub.Publish(msg.Context(), &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 8, Email: "follow-up@example.com"})
		})
	})
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { cleanup.Report("run the router", router.Run(runCtx)) }()
	<-router.Running()

	publishing, span := otel.Tracer("test").Start(context.Background(), "publish")
	require.NoError(t, bus.WithinTx(publishing, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(publishing, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: "chain@example.com"})
	}))
	span.End()

	first := testhelper.Receive(t, handled, "the event was handled")
	followUp := testhelper.Receive(t, handled, "the event its handler published was handled")
	assert.Equal(t, first.CorrelationID, followUp.CorrelationID)
	assert.Equal(t, first.ID, followUp.CausationID)
	assert.True(t, linkedTo(spans.Ended(), "process chain", span.SpanContext().TraceID().String()), "the handler's span links to the publisher's")
}

// linkedTo reports whether a span of that name links to a span of the trace.
func linkedTo(spans []sdktrace.ReadOnlySpan, name, traceID string) bool {
	for _, s := range spans {
		for _, l := range s.Links() {
			if s.Name() == name && l.SpanContext.TraceID().String() == traceID {
				return true
			}
		}
	}
	return false
}

// publishIn publishes a ContactCreated in ctx and returns its envelope as the outbox holds it.
func publishIn(ctx context.Context, t *testing.T, env *testhelper.TestEnv, email string) events.Envelope {
	t.Helper()
	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.ContactCreated{WorkspaceID: fixtures.AcmeID, ContactID: 7, Email: email})
	}))
	envelopes, err := testhelper.OutboxEnvelopesOn(ctx, env.DB, events.NameContactCreated)
	require.NoError(t, err)
	return envelopes[len(envelopes)-1]
}

type chainedArgs struct{}

func (chainedArgs) Kind() string { return "chained" }

// A job queued while handling an event keeps the event's chain, and an event the job publishes,
// even in a context of its own, is caused by the handled one.
func TestAJobKeepsTheChainItWasQueuedIn(t *testing.T) {
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	bus := events.New(sqlDB)
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
	published, err := testhelper.OutboxEnvelopesOn(ctx, db.NewEntClient(sqlDB), events.NameContactCreated)
	require.NoError(t, err)
	require.NotEmpty(t, published)
	assert.Equal(t, "01JROOT", published[len(published)-1].CorrelationID)
	assert.Equal(t, "01JPARENT", published[len(published)-1].CausationID)
}
