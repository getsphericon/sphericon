package events

import (
	"context"
	"fmt"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/schema"
)

// PruneEvents deletes analytical Events created more than retention ago, in
// batches of batchSize, and returns how many it removed. Age is created_at (the
// insert time), not occurred_at: created_at is never null and is what the table's
// growth follows; an Event backfilled with an old occurred_at is kept until its
// row itself ages out. A retention of zero or less disables the prune. The job is
// idempotent: a rerun finds nothing left to delete.
func PruneEvents(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)
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

// PruneRuleFirings deletes Rule Firings created more than retention ago, in batches of
// batchSize, and returns how many it removed: they are analytical history of what a Rule did,
// kept for the same window as the Events that fired them (ADR 0019, ADR 0054). A retention of
// zero or less disables the prune.
func PruneRuleFirings(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)
	var total int64
	for {
		ids, err := client.RuleFiring.Query().
			Where(rulefiring.CreatedAtLT(cutoff)).
			Order(rulefiring.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return total, fmt.Errorf("select expired rule firings batch: %w", err)
		}
		if len(ids) == 0 {
			return total, nil
		}
		n, err := client.RuleFiring.Delete().Where(rulefiring.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return total, fmt.Errorf("delete expired rule firings batch: %w", err)
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
