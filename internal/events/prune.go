package events

import (
	"context"
	"fmt"
	"slices"
	"time"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/outboxoffset"
)

// PruneResult reports one PruneOutbox run.
type PruneResult struct {
	// Deleted is the number of outbox rows removed.
	Deleted int64
	// RetiredGroups is the number of offsets rows removed for consumer groups no
	// longer registered in code.
	RetiredGroups int64
	// Skipped is true when the run deleted no outbox rows because a registered
	// consumer group has no usable cursor yet (no offsets row, or a NULL ack).
	Skipped bool
}

// PruneOutbox deletes outbox rows in batches of batchSize that sort strictly
// below the lowest registered consumer group's cursor, by the same
// (transaction_id, offset) tuple the subscribers read in, and are older than
// floor (ADR 0019). The row at the cursor is kept so an at-least-once redelivery
// still finds its place; the events table is the durable record, and its unique
// source_id dedupes a redelivered message.
//
// It first drops the offsets rows of groups that are no longer registered in
// code (ConsumerGroups): a retired group's frozen cursor would otherwise pin the
// outbox forever. It then skips the run when any registered group has no offsets
// row or a NULL ack, because that group has not consumed anything and may still
// need every row.
func PruneOutbox(ctx context.Context, client *ent.Client, floor time.Duration, batchSize int) (PruneResult, error) {
	var res PruneResult
	groups := ConsumerGroups()

	retired, err := client.OutboxOffset.Delete().Where(outboxoffset.IDNotIn(groups...)).Exec(ctx)
	if err != nil {
		return res, fmt.Errorf("remove retired consumer groups: %w", err)
	}
	res.RetiredGroups = int64(retired)

	// The registered groups' cursors, lowest first: the first row bounds the prune.
	cursors, err := client.OutboxOffset.Query().
		Where(outboxoffset.IDIn(groups...)).
		Order(outboxoffset.ByLastProcessedTransactionID(), outboxoffset.ByOffsetAcked(entsql.OrderNullsFirst())).
		All(ctx)
	if err != nil {
		return res, fmt.Errorf("read consumer cursors: %w", err)
	}
	incomplete := slices.ContainsFunc(cursors, func(c *ent.OutboxOffset) bool { return c.OffsetAcked == nil })
	if incomplete || len(cursors) != len(groups) {
		res.Skipped = true
		return res, nil
	}
	lowest := cursors[0]

	cutoff := time.Now().Add(-floor)
	for {
		ids, err := client.OutboxMessage.Query().
			Where(outboxBefore(lowest.LastProcessedTransactionID, *lowest.OffsetAcked), outboxmessage.CreatedAtLT(cutoff)).
			Order(outboxmessage.ByTransactionID(), outboxmessage.ByID()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return res, fmt.Errorf("select outbox batch: %w", err)
		}
		if len(ids) == 0 {
			return res, nil
		}
		n, err := client.OutboxMessage.Delete().Where(outboxmessage.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return res, fmt.Errorf("delete outbox batch: %w", err)
		}
		res.Deleted += int64(n)
		if len(ids) < batchSize {
			return res, nil
		}
	}
}
