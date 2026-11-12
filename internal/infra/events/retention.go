package events

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/eventreceipt"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/schema"
)

// PruneEvents deletes analytical Events created more than retention ago, in
// batches of batchSize, and returns how many it removed. Age is created_at (the
// insert time), not occurred_at: created_at is never null and is what the table's
// growth follows; an Event backfilled with an old occurred_at is kept until its
// row itself ages out. A retention of zero or less disables the prune. The job is
// idempotent: a rerun finds nothing left to delete.
func PruneEvents(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int, now time.Time) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-retention)
	var total int64
	for {
		ids, err := client.Event.Query().
			Where(event.CreatedAtLT(cutoff), expirable()).
			Order(event.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return total, fmt.Errorf("select expired events batch: %w", err)
		}
		if len(ids) == 0 {
			return total, nil
		}
		n, err := client.Event.Delete().Where(event.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("delete expired events batch: %w", err)
		}
		total += int64(n)
		if len(ids) < batchSize {
			return total, nil
		}
	}
}

// expirable is schema.ExpirableEventPredicate on the generated query: the same text the
// partial index is declared with, so the delete can always use that index.
func expirable() predicate.Event {
	return predicate.Event(func(s *sql.Selector) {
		// raw sql: the text of the partial index's WHERE, so the planner can use that index; an ent
		// predicate would render differently and miss it.
		s.Where(sql.P(func(b *sql.Builder) { b.WriteString(schema.ExpirableEventPredicate) }))
	})
}

// PruneReceipts deletes the receipts of external facts (ADR 0114) created more than retention ago,
// in batches of batchSize, and returns how many it removed. A receipt only rejects a redelivered
// fact, and the Events' own deduplication by source_id ends with the same window, so a fact older
// than it may be consumed again: a provider redelivers within days, never past the window. A
// receipt holds a digest and no Workspace, so this age is its only end. A retention of zero or less
// disables the prune.
func PruneReceipts(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int, now time.Time) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-retention)
	var total int64
	for {
		ids, err := client.EventReceipt.Query().
			Where(eventreceipt.CreatedAtLT(cutoff)).
			Order(eventreceipt.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return total, fmt.Errorf("select expired receipts batch: %w", err)
		}
		if len(ids) == 0 {
			return total, nil
		}
		n, err := client.EventReceipt.Delete().Where(eventreceipt.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("delete expired receipts batch: %w", err)
		}
		total += int64(n)
		if len(ids) < batchSize {
			return total, nil
		}
	}
}
