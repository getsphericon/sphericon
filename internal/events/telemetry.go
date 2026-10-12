package events

import (
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const otelScope = "github.com/getsphericon/sphericon/internal/events"

// otelMiddleware is a watermill router middleware that opens a span and records
// processed-count + handler-duration metrics per domain-event handler, using
// the global OTel providers set by telemetry.Setup (a no-op when telemetry is
// disabled, e.g. tests). Instruments are built once and shared across handlers.
func otelMiddleware() message.HandlerMiddleware {
	tracer := otel.Tracer(otelScope)
	meter := otel.Meter(otelScope)
	processed, _ := meter.Int64Counter(
		"watermill.messages.processed",
		metric.WithDescription("Domain-event messages processed, by handler and outcome."),
	)
	duration, _ := meter.Float64Histogram(
		"watermill.handler.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Domain-event handler execution time."),
	)

	return func(next message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			handler := message.HandlerNameFromCtx(msg.Context())
			ctx, span := tracer.Start(msg.Context(), "process "+handler,
				trace.WithSpanKind(trace.SpanKindConsumer),
				trace.WithAttributes(
					attribute.String("messaging.system", "watermill"),
					attribute.String("messaging.destination.name", TopicDomainEvents),
					attribute.String("messaging.handler", handler),
					attribute.String("messaging.message.id", msg.UUID),
				),
			)
			msg.SetContext(ctx)

			start := time.Now()
			out, err := next(msg)
			attrs := metric.WithAttributes(
				attribute.String("handler", handler),
				attribute.Bool("error", err != nil),
			)
			duration.Record(ctx, time.Since(start).Seconds(), attrs)
			processed.Add(ctx, 1, attrs)

			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.End()
			return out, err
		}
	}
}
