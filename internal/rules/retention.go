package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rulefiring"
)

// PruneFirings deletes Rule Firings created more than retention ago, in batches of
// batchSize, and returns how many it removed: they are analytical history of what a Rule did,
// kept for the same window as the Events that fired them (ADR 0019, ADR 0054), and the Event
// retention job prunes both. A retention of zero or less disables the prune. It spans every
// Workspace, so it takes the raw client.
func PruneFirings(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
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
