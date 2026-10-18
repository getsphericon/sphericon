package events

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/outboxoffset"
)

// outboxLag is the age in seconds of the oldest outbox row past a consumer group's
// cursor, 0 when the group is caught up. The cursor is the (last_processed_transaction_id,
// offset_acked) pair the watermill offsets adapter keeps; a group with no offsets row yet
// has not consumed anything, so its cursor is the start of the table. The outbox has no
// published flag, so "past the cursor" is the only definition of pending.
//
// created_at is timestamptz (InitSchema converts watermill's plain TIMESTAMP), so the
// difference is absolute, independent of any session TimeZone.
func outboxLag(ctx context.Context, client *ent.Client, group string, now time.Time) (float64, error) {
	transactionID, acked := "0", int64(0)
	cursor, err := client.OutboxOffset.Query().Where(outboxoffset.ID(group)).Only(ctx)
	switch {
	case ent.IsNotFound(err):
	case err != nil:
		return 0, err
	default:
		transactionID = cursor.LastProcessedTransactionID
		if cursor.OffsetAcked != nil {
			acked = *cursor.OffsetAcked
		}
	}
	oldest, err := client.OutboxMessage.Query().
		Where(outboxAfter(transactionID, acked)).
		Order(outboxmessage.ByCreatedAt()).
		First(ctx)
	if ent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return now.Sub(oldest.CreatedAt).Seconds(), nil
}

// RegisterLagGauge exports outbox.lag (seconds, one sample per consumer group in
// ConsumerGroups) on the global meter provider. The label is the group name only:
// a bounded technical dimension. The reads run once per scrape. The returned
// function unregisters the callback.
func RegisterLagGauge(client *ent.Client) (func() error, error) {
	meter := otel.Meter(otelScope)
	gauge, err := meter.Float64ObservableGauge(
		"outbox.lag",
		metric.WithUnit("s"),
		metric.WithDescription("Age of the oldest domain-event outbox row past each consumer group's cursor."),
	)
	if err != nil {
		return nil, fmt.Errorf("outbox lag gauge: %w", err)
	}
	reg, err := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		now := time.Now()
		for _, group := range ConsumerGroups() {
			lag, err := outboxLag(ctx, client, group, now)
			if err != nil {
				return fmt.Errorf("outbox lag %s: %w", group, err)
			}
			o.ObserveFloat64(gauge, lag, metric.WithAttributes(attribute.String("consumer_group", group)))
		}
		return nil
	}, gauge)
	if err != nil {
		return nil, fmt.Errorf("outbox lag callback: %w", err)
	}
	return reg.Unregister, nil
}
