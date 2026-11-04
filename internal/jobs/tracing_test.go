package jobs_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type tracedArgs struct{}

func (tracedArgs) Kind() string { return "traced" }

// A job inserted in a traced context carries that trace context, and the span of its work links
// to the span that inserted it.
func TestAJobLinksToTheSpanThatInsertedIt(t *testing.T) {
	testhelper.IsolateOtel(t)
	spans := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)))
	ctx := context.Background()
	sqlDB := testhelper.RealDB(t)
	inserter, err := jobs.NewInserter(sqlDB)
	require.NoError(t, err)
	tx, err := sqlDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	inserting, span := otel.Tracer("test").Start(ctx, "insert")
	queued, err := inserter.InsertTx(inserting, tx, tracedArgs{}, nil)
	span.End()
	require.NoError(t, err)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(queued.Job.Metadata, &metadata))
	assert.Contains(t, metadata, "traceparent", "the job carries the trace context it was inserted in")

	w := rivertest.NewWorker(t, riverdatabasesql.New(sqlDB), jobs.BaseConfig(), river.WorkFunc(func(context.Context, *river.Job[tracedArgs]) error {
		return nil
	}))
	_, err = w.WorkJob(ctx, t, tx, queued.Job)
	require.NoError(t, err)

	assert.True(t, linksTo(spans.Ended(), "river.work", span.SpanContext().TraceID().String()), "the work span links to the inserting span")
}

// linksTo reports whether a span of that name links to a span of the trace.
func linksTo(spans []sdktrace.ReadOnlySpan, name, traceID string) bool {
	for _, s := range spans {
		for _, l := range s.Links() {
			if s.Name() == name && l.SpanContext.TraceID().String() == traceID {
				return true
			}
		}
	}
	return false
}
