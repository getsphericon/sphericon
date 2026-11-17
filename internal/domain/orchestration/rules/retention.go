package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/internal/kernel/drain"
)

// PruneFirings deletes Rule Firings created more than retention before the Instance clock's now,
// in batches of batchSize, and returns how many it removed: they are analytical history of what a
// Rule did, kept for the same window as the Events that fired them (ADR 0019, ADR 0054), and the
// Event retention job prunes both. A retention of zero or less disables the prune. It spans every
// Workspace, so it takes the raw client.
func (m *Module) PruneFirings(ctx context.Context, client *ent.Client, retention time.Duration, batchSize int) (int64, error) {
	if retention <= 0 {
		return 0, nil
	}
	cutoff := m.now().Add(-retention)
	total, err := drain.Batches(batchSize, func() (int, error) {
		ids, err := client.RuleFiring.Query().
			Where(rulefiring.CreatedAtLT(cutoff)).
			Order(rulefiring.ByCreatedAt()).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("select expired rule firings batch: %w", err)
		}
		if len(ids) == 0 {
			return 0, nil
		}
		n, err := client.RuleFiring.Delete().Where(rulefiring.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("delete expired rule firings batch: %w", err)
		}
		return n, nil
	})
	return int64(total), err
}
