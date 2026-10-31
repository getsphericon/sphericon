package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/ThreeDotsLabs/watermill"
	watermillsql "github.com/ThreeDotsLabs/watermill-sql/v4/pkg/sql"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/ThreeDotsLabs/watermill/message/router/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboxdeadletter"
	"github.com/getsphericon/sphericon/ent/predicate"
)

// A consumer group's handler that fails on an event is retried with backoff (Retry) and, past
// its retries, the event is published to TopicDeadLetters (watermill's PoisonQueue) and acked,
// so the group moves on instead of redelivering it forever. A panic is recovered into an error
// first (Recoverer). Every group shares the outbox topic, so a dead letter is not published back
// to it: it waits in its own table, with its group and its error, until an operator replays it
// to that group (ReplayDeadLetters, `sphericon events replay`).

// TopicDeadLetters is the topic of the dead letters: watermill-sql's table
// "watermill_domain_events_dead_letters" (migrations), read through ent as OutboxDeadLetter.
const TopicDeadLetters = "domain_events_dead_letters"

// MetadataConsumerGroup is the metadata key that names, on a dead letter, the consumer group
// whose handler failed on it. The error is under middleware.ReasonForPoisonedKey.
const MetadataConsumerGroup = "consumer_group"

// RouterOption adjusts the router NewRouter builds.
type RouterOption func(*routerConfig)

type routerConfig struct {
	retry middleware.Retry
}

// WithRetry replaces the retry policy of a failing handler (tests make it short).
func WithRetry(r middleware.Retry) RouterOption {
	return func(c *routerConfig) { c.retry = r }
}

// defaultRetry retries a failing handler for about four minutes before the event is
// dead-lettered: long enough to ride out a deploy or a short outage of what the handler calls,
// short enough that one bad event does not hold its group for long. A database outage holds
// the group anyway: the dead letter cannot be written either, so the event is redelivered.
func defaultRetry() middleware.Retry {
	return middleware.Retry{
		MaxRetries:          8,
		InitialInterval:     time.Second,
		MaxInterval:         time.Minute,
		Multiplier:          2,
		RandomizationFactor: 0.2,
		Logger:              watermill.NewSlogLogger(slog.Default()),
	}
}

// deadLetters is the router middleware that publishes an event a handler failed on past its
// retries to TopicDeadLetters. A handler stopped by shutdown is not a failure of the event: it
// is nacked and redelivered.
func deadLetters(db *sql.DB) (message.HandlerMiddleware, error) {
	pub, err := watermillsql.NewPublisher(watermillsql.BeginnerFromStdSQL(db), watermillsql.PublisherConfig{
		SchemaAdapter: outboxSchema,
	}, watermill.NewSlogLogger(slog.Default()))
	if err != nil {
		return nil, fmt.Errorf("dead letter publisher: %w", err)
	}
	return middleware.PoisonQueueWithFilter(pub, TopicDeadLetters, func(err error) bool {
		return !errors.Is(err, context.Canceled)
	})
}

// inGroup marks the message with its consumer group, so a dead letter keeps it.
func inGroup(group string) message.HandlerMiddleware {
	return func(h message.HandlerFunc) message.HandlerFunc {
		return func(msg *message.Message) ([]*message.Message, error) {
			msg.Metadata.Set(MetadataConsumerGroup, group)
			return h(msg)
		}
	}
}

// ofGroup selects the dead letters of a consumer group.
func ofGroup(group string) predicate.OutboxDeadLetter {
	return predicate.OutboxDeadLetter(func(s *entsql.Selector) {
		s.Where(sqljson.ValueEQ(s.C(outboxdeadletter.FieldMetadata), group, sqljson.Path(MetadataConsumerGroup)))
	})
}

// ReplayDeadLetters runs the dead letters of group through handle, oldest first, and deletes
// each one handle accepts. It stops at the first that fails again, which stays, and returns how
// many were replayed. handle is the group's own handler (ConsumerHandlers), so the other groups,
// which consumed the event already, do not see it twice.
func ReplayDeadLetters(ctx context.Context, client *ent.Client, group string, handle message.NoPublishHandlerFunc) (int, error) {
	letters, err := client.OutboxDeadLetter.Query().Where(ofGroup(group)).Order(outboxdeadletter.ByID()).All(ctx)
	if err != nil {
		return 0, err
	}
	for i, l := range letters {
		msg := message.NewMessage(l.UUID, l.Payload)
		// The message is rebuilt whole: its metadata may carry the envelope's header.
		if len(l.Metadata) > 0 {
			if err := json.Unmarshal(l.Metadata, &msg.Metadata); err != nil {
				return i, fmt.Errorf("dead letter %s metadata: %w", l.UUID, err)
			}
		}
		msg.SetContext(ctx)
		if err := handle(msg); err != nil {
			return i, fmt.Errorf("replay dead letter %s of %s: %w", l.UUID, group, err)
		}
		if err := client.OutboxDeadLetter.DeleteOneID(l.ID).Exec(ctx); err != nil {
			return i, err
		}
	}
	return len(letters), nil
}

// RegisterDeadLetterGauge exports outbox.dead_letters (one sample per consumer group in
// ConsumerGroups) on the global meter provider: the events each group's handler gave up on and
// that wait to be replayed. The alert fires on any. The returned function unregisters it.
func RegisterDeadLetterGauge(client *ent.Client) (func() error, error) {
	meter := otel.Meter(otelScope)
	gauge, err := meter.Int64ObservableGauge(
		"outbox.dead_letters",
		metric.WithDescription("Domain events a consumer group's handler gave up on, waiting to be replayed to it."),
	)
	if err != nil {
		return nil, fmt.Errorf("dead letter gauge: %w", err)
	}
	reg, err := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		for _, group := range ConsumerGroups() {
			n, err := client.OutboxDeadLetter.Query().Where(ofGroup(group)).Count(ctx)
			if err != nil {
				return fmt.Errorf("dead letters of %s: %w", group, err)
			}
			o.ObserveInt64(gauge, int64(n), metric.WithAttributes(attribute.String("consumer_group", group)))
		}
		return nil
	}, gauge)
	if err != nil {
		return nil, fmt.Errorf("dead letter callback: %w", err)
	}
	return reg.Unregister, nil
}
