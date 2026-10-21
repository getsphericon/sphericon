package jobs

import (
	"context"
	"database/sql"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const otelScope = "github.com/getsphericon/sphericon/internal/jobs"

// queueStatsQuery counts the jobs a worker could run right now (available, due)
// per queue and the age of the oldest of them.
const queueStatsQuery = `
SELECT queue,
       count(*),
       EXTRACT(EPOCH FROM (now() - min(scheduled_at)))::float8
FROM river_job
WHERE state = 'available' AND scheduled_at <= now()
GROUP BY queue`

// RegisterQueueMetrics exports river.queue.depth (jobs ready to run) and
// river.queue.oldest_available_age (seconds) per queue on the global meter
// provider. The only label is the queue name, a bounded technical dimension.
// A queue with no ready jobs reports no sample. The query runs once per scrape.
// The returned function unregisters the callback.
func RegisterQueueMetrics(riverDB *sql.DB) (func() error, error) {
	meter := otel.Meter(otelScope)
	depth, err := meter.Int64ObservableGauge(
		"river.queue.depth",
		metric.WithDescription("Jobs available to run now, by queue."),
	)
	if err != nil {
		return nil, fmt.Errorf("queue depth gauge: %w", err)
	}
	oldest, err := meter.Float64ObservableGauge(
		"river.queue.oldest_available_age",
		metric.WithUnit("s"),
		metric.WithDescription("Age of the oldest available job, by queue."),
	)
	if err != nil {
		return nil, fmt.Errorf("queue age gauge: %w", err)
	}
	reg, err := meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		rows, err := riverDB.QueryContext(ctx, queueStatsQuery)
		if err != nil {
			return fmt.Errorf("queue stats: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var queue string
			var n int64
			var age float64
			if err := rows.Scan(&queue, &n, &age); err != nil {
				return fmt.Errorf("queue stats scan: %w", err)
			}
			attrs := metric.WithAttributes(attribute.String("queue", queue))
			o.ObserveInt64(depth, n, attrs)
			o.ObserveFloat64(oldest, age, attrs)
		}
		return rows.Err()
	}, depth, oldest)
	if err != nil {
		return nil, fmt.Errorf("queue stats callback: %w", err)
	}
	return reg.Unregister, nil
}
