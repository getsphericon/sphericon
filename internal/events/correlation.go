package events

import (
	"context"
	"encoding/json"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"github.com/oklog/ulid/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/getsphericon/sphericon/internal/logging"
)

// Every event carries a correlation id, shared by the whole chain it belongs to, and, when it was
// published while another was being handled, a causation id: the id of that event. A chain
// starts at a request (its request id) or, outside one, at a fresh id. The ids travel as message
// metadata, and through the context from a handler to what it publishes, directly or through a
// river job it queues (JobCorrelation). The OpenTelemetry trace context of the publisher travels
// in the metadata too, and the handler's span is linked to it.

// The metadata keys of the chain an event belongs to. The correlation id's is watermill's own.
const (
	MetaCorrelationID = middleware.CorrelationIDMetadataKey
	MetaCausationID   = "causation_id"
)

type chainKey struct{}

// chain is what the context of a handler, or of a job queued by one, says an event published in
// it follows.
type chain struct {
	Correlation string `json:"correlation_id"`
	Cause       string `json:"causation_id,omitempty"`
}

// withChain returns ctx carrying c, and tags the logs written with it with the correlation id.
func withChain(ctx context.Context, c chain) context.Context {
	return logging.WithCorrelationID(context.WithValue(ctx, chainKey{}, c), c.Correlation)
}

// chainOf is the chain ctx carries; outside a handler, a new one rooted at the request, if any.
func chainOf(ctx context.Context) chain {
	if c, ok := ctx.Value(chainKey{}).(chain); ok {
		return c
	}
	if id, ok := logging.RequestIDFromContext(ctx); ok && id != "" {
		return chain{Correlation: id}
	}
	return chain{Correlation: ulid.Make().String()}
}

// Handling is ctx for handling env: what is published in it is caused by env and shares its
// correlation id. The router does it for every handler (correlationMiddleware).
func Handling(ctx context.Context, env Envelope) context.Context {
	correlation := env.CorrelationID
	if correlation == "" {
		correlation = env.ID // published before events carried one: the chain starts here
	}
	return withChain(ctx, chain{Correlation: correlation, Cause: env.ID})
}

// stamp sets the chain of ctx on env.
func stamp(ctx context.Context, env *Envelope) {
	c := chainOf(ctx)
	env.CorrelationID, env.CausationID = c.Correlation, c.Cause
}

// injectTrace puts the trace context of ctx in msg's metadata, for the handler's span to link to.
func injectTrace(ctx context.Context, msg *message.Message) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.Metadata))
}

// correlationMiddleware makes the context of every handler that of handling its event (Handling).
func correlationMiddleware(next message.HandlerFunc) message.HandlerFunc {
	return func(msg *message.Message) ([]*message.Message, error) {
		env, err := EnvelopeOf(msg)
		if err != nil {
			return next(msg) // the handler reports the undecodable message itself
		}
		msg.SetContext(Handling(msg.Context(), env))
		return next(msg)
	}
}

// JobCorrelation is the river middleware that carries the chain of an event through the jobs
// queued while handling it: a job inserted in a handler's context keeps the chain in its
// metadata, and the job's work runs in it, so an event the job publishes is caused by the same
// event.
func JobCorrelation() *JobChain { return &JobChain{} }

// JobChain is JobCorrelation's middleware: river's insert and worker middleware both.
type JobChain struct {
	river.MiddlewareDefaults
}

var (
	_ rivertype.JobInsertMiddleware = (*JobChain)(nil)
	_ rivertype.WorkerMiddleware    = (*JobChain)(nil)
)

// InsertMany records the chain of ctx, when it has one, in each job's metadata.
func (*JobChain) InsertMany(ctx context.Context, params []*rivertype.JobInsertParams, doInner func(context.Context) ([]*rivertype.JobInsertResult, error)) ([]*rivertype.JobInsertResult, error) {
	c, ok := ctx.Value(chainKey{}).(chain)
	if !ok {
		return doInner(ctx)
	}
	for _, p := range params {
		metadata := map[string]any{}
		if len(p.Metadata) > 0 {
			if err := json.Unmarshal(p.Metadata, &metadata); err != nil {
				return nil, err
			}
		}
		metadata[MetaCorrelationID] = c.Correlation
		metadata[MetaCausationID] = c.Cause
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}
		p.Metadata = encoded
	}
	return doInner(ctx)
}

// Work runs the job in the chain its metadata recorded, if any.
func (*JobChain) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	var c chain
	if err := json.Unmarshal(job.Metadata, &c); err != nil || c.Correlation == "" {
		return doInner(ctx)
	}
	return doInner(withChain(ctx, c))
}
