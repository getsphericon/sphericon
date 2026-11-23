package events

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/oklog/ulid/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/getsphericon/sphericon/internal/infra/logging"
)

// Every event carries a correlation id, shared by the whole chain it belongs to, and, when it was
// published while another was being handled, a causation id: the id of that event. A chain
// starts at a request (its request id) or, outside one, at a fresh id. The ids travel in the
// envelope, and through the context from a consumer to what it publishes, directly or through a
// river job it queues (JobCorrelation). The OpenTelemetry trace context travels in each job's
// metadata (otelriver's trace propagation), and the span of its work is linked to it.
//
// The chain also names the Automations it passed through (ADR 0127): what is published on behalf of
// an Enrollment carries its Automation and the ones before it (PassingThrough), what is published
// while handling such an event inherits them, and they cross jobs as the ids do. An Automation's
// entry checks read them to skip an entry whose cause already passed through it (`cycle`). They
// never age: a chain that waited months still names them.

// The job metadata keys of the chain a job was queued in.
const (
	metaCorrelationID = "correlation_id"
	metaCausationID   = "causation_id"
	metaAutomations   = "automations"
)

type chainKey struct{}

// chain is what the context of a handler, or of a job queued by one, says an event published in
// it follows.
type chain struct {
	Correlation string `json:"correlation_id"`
	Cause       string `json:"causation_id,omitempty"`
	// Automations are the ids of the Automations the chain passed through, ascending, no repeats.
	Automations []int64 `json:"automations,omitempty"`
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
// correlation id. JobChain does it for every event consumer's job.
func Handling(ctx context.Context, env Envelope) context.Context {
	correlation := env.CorrelationID
	if correlation == "" {
		correlation = env.ID // published before events carried one: the chain starts here
	}
	return withChain(ctx, chain{Correlation: correlation, Cause: env.ID, Automations: env.Automations})
}

// PassingThrough is ctx for publishing on behalf of an Enrollment: what is published in it carries
// the Automations of ctx's chain and automationIDs besides (Envelope.Automations), and keeps its
// correlation and its cause. Outside a chain it starts one, as chainOf does.
func PassingThrough(ctx context.Context, automationIDs ...int64) context.Context {
	c := chainOf(ctx)
	c.Automations = AutomationSet(c.Automations, automationIDs...)
	return withChain(ctx, c)
}

// AutomationsOf is the Automations the chain of ctx passed through: those of the event being
// handled, in a consumer's job or what it queued, and of the Enrollment it is published for.
func AutomationsOf(ctx context.Context) []int64 {
	c, _ := ctx.Value(chainKey{}).(chain)
	return c.Automations
}

// AutomationSet is the Automations of ids and more together, ascending and without repeats, in a
// new slice: the list a chain carries.
func AutomationSet(ids []int64, more ...int64) []int64 {
	set := slices.Concat(ids, more)
	slices.Sort(set)
	return slices.Clip(slices.Compact(set))
}

// stamp sets the chain of ctx on env.
func stamp(ctx context.Context, env *Envelope) {
	c := chainOf(ctx)
	env.CorrelationID, env.CausationID, env.Automations = c.Correlation, c.Cause, c.Automations
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

// InsertMany records the chain of ctx, when it has one, in each job's metadata. A ctx that carries
// Automations always has a chain (Handling, PassingThrough), so none is dropped.
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
		metadata[metaCorrelationID] = c.Correlation
		metadata[metaCausationID] = c.Cause
		if len(c.Automations) > 0 {
			metadata[metaAutomations] = c.Automations
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}
		p.Metadata = encoded
	}
	return doInner(ctx)
}

// Work runs the job in the chain its metadata recorded, if any. A consumer's job is inserted in
// the context of handling its event (Publish queues it under Handling), so its metadata records
// that event as the cause: what the job publishes is caused by the event it handles.
func (*JobChain) Work(ctx context.Context, job *rivertype.JobRow, doInner func(context.Context) error) error {
	var c chain
	if err := json.Unmarshal(job.Metadata, &c); err != nil || c.Correlation == "" {
		return doInner(ctx)
	}
	return doInner(withChain(ctx, c))
}
